package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"peanut/internal/intent"
	"peanut/internal/storage/sqlite"
)

const HomeAssistantProviderID = "homeassistant"

var (
	ErrProviderUnavailable  = errors.New("provider_unavailable")
	ErrProviderUnauthorized = errors.New("provider_unauthorized")
	ErrProviderRejected     = errors.New("provider_rejected")
	ErrInvalidAction        = errors.New("invalid_action")
)

type HomeAssistantProvider struct {
	config       *sqlite.ConfigStore
	capabilities *sqlite.CapabilityStore
	client       *http.Client
}

func NewHomeAssistantProvider(db *sqlite.DB, client *http.Client) *HomeAssistantProvider {
	if client == nil {
		client = http.DefaultClient
	}
	return &HomeAssistantProvider{
		config:       sqlite.NewConfigStore(db),
		capabilities: sqlite.NewCapabilityStore(db),
		client:       client,
	}
}

func (*HomeAssistantProvider) Descriptor() ProviderDescriptor {
	return ProviderDescriptor{ID: HomeAssistantProviderID, Type: "homeassistant", Name: "Home Assistant"}
}

func (p *HomeAssistantProvider) Discover(ctx context.Context) ([]intent.Capability, error) {
	settings, err := p.config.HomeAssistant(ctx)
	if err != nil {
		return nil, err
	}
	var states []struct {
		EntityID   string `json:"entity_id"`
		Attributes struct {
			FriendlyName string `json:"friendly_name"`
		} `json:"attributes"`
	}
	if err := p.request(ctx, settings, http.MethodGet, "/api/states", nil, &states); err != nil {
		if errors.Is(err, ErrProviderUnavailable) {
			cached, cacheErr := p.capabilities.List(ctx, HomeAssistantProviderID)
			if cacheErr == nil && len(cached) > 0 {
				return cached, err
			}
		}
		return nil, err
	}
	capabilities := make([]intent.Capability, 0, len(states))
	for _, state := range states {
		if !strings.HasPrefix(state.EntityID, "media_player.") {
			continue
		}
		name := state.Attributes.FriendlyName
		if name == "" {
			name = state.EntityID
		}
		capabilities = append(capabilities, intent.Capability{
			ID:         HomeAssistantProviderID + "." + state.EntityID,
			ProviderID: HomeAssistantProviderID,
			DeviceID:   state.EntityID,
			Type:       "music",
			Name:       name,
			Actions: []intent.ActionDefinition{{
				ID:           "music.play",
				ExactlyOneOf: []string{"query", "uri"},
				Arguments: map[string]intent.ArgumentDefinition{
					"query":  {Type: intent.TypeString},
					"uri":    {Type: intent.TypeString},
					"source": {Type: intent.TypeString},
				},
			}},
		})
	}
	sort.Slice(capabilities, func(i, j int) bool { return capabilities[i].ID < capabilities[j].ID })
	if err := p.capabilities.Replace(ctx, HomeAssistantProviderID, "homeassistant", capabilities); err != nil {
		return nil, err
	}
	return capabilities, nil
}

func (p *HomeAssistantProvider) Execute(ctx context.Context, action intent.ActionRequest) (ActionResult, error) {
	payload, err := playMediaPayload(action)
	if err != nil {
		return ActionResult{}, err
	}
	settings, err := p.config.HomeAssistant(ctx)
	if err != nil {
		return ActionResult{}, err
	}
	if err := p.request(ctx, settings, http.MethodPost, "/api/services/media_player/play_media", payload, nil); err != nil {
		return ActionResult{}, err
	}
	return ActionResult{}, nil
}

func playMediaPayload(action intent.ActionRequest) (map[string]any, error) {
	if !strings.HasPrefix(action.DeviceID, "media_player.") || action.ActionID != "music.play" {
		return nil, fmt.Errorf("%w: unsupported device or action", ErrInvalidAction)
	}
	values := make(map[string]string, len(action.Arguments))
	for name, value := range action.Arguments {
		if name != "query" && name != "uri" && name != "source" {
			return nil, fmt.Errorf("%w: unknown argument %q", ErrInvalidAction, name)
		}
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("%w: argument %q must be a non-empty string", ErrInvalidAction, name)
		}
		values[name] = text
	}
	query, uri := values["query"], values["uri"]
	if (query == "") == (uri == "") {
		return nil, fmt.Errorf("%w: exactly one of query or uri is required", ErrInvalidAction)
	}
	contentID := query
	if uri != "" {
		contentID = uri
	}
	contentType := values["source"]
	if contentType == "" {
		contentType = "music"
	}
	return map[string]any{
		"entity_id":          action.DeviceID,
		"media_content_id":   contentID,
		"media_content_type": contentType,
	}, nil
}

func (p *HomeAssistantProvider) request(ctx context.Context, settings sqlite.HomeAssistantConfig, method, path string, body any, destination any) error {
	requestCtx, cancel := context.WithTimeout(ctx, settings.Timeout)
	defer cancel()
	var encoded bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&encoded).Encode(body); err != nil {
			return fmt.Errorf("encode Home Assistant request: %w", err)
		}
	}
	base, err := url.Parse(settings.URL)
	if err != nil {
		return fmt.Errorf("invalid Home Assistant URL: %w", err)
	}
	base.Path = strings.TrimRight(base.Path, "/") + path
	req, err := http.NewRequestWithContext(requestCtx, method, base.String(), &encoded)
	if err != nil {
		return fmt.Errorf("create Home Assistant request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+settings.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: Home Assistant request failed", ErrProviderUnavailable)
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: Home Assistant rejected credentials", ErrProviderUnauthorized)
	case response.StatusCode >= 500:
		return fmt.Errorf("%w: Home Assistant returned %s", ErrProviderUnavailable, response.Status)
	case response.StatusCode < 200 || response.StatusCode >= 300:
		return fmt.Errorf("%w: Home Assistant returned %s", ErrProviderRejected, response.Status)
	}
	if destination != nil {
		if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
			return fmt.Errorf("decode Home Assistant response: %w", err)
		}
	}
	return nil
}
