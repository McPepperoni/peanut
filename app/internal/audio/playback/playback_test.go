package playback

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"peanut/internal/audio"
)

func TestPlayWAVRoutesDecodedAudioToPlayer(t *testing.T) {
	want, err := audio.NewAudio(audio.SampleRate, audio.Channels, []float32{0, 0.5, -0.5})
	if err != nil {
		t.Fatal(err)
	}
	var wav bytes.Buffer
	if err := audio.WriteWAV(&wav, want); err != nil {
		t.Fatal(err)
	}
	player := &Fake{}
	if err := PlayWAV(context.Background(), player, bytes.NewReader(wav.Bytes())); err != nil {
		t.Fatal(err)
	}
	if len(player.Played) != 1 || len(player.Played[0].Samples) != len(want.Samples) {
		t.Fatalf("played = %+v", player.Played)
	}
}

func TestSystemPlayerIsUnsupported(t *testing.T) {
	var player audio.Player = SystemPlayer{}
	input, err := audio.NewAudio(audio.SampleRate, audio.Channels, []float32{0})
	if err != nil {
		t.Fatal(err)
	}
	if err := player.Play(context.Background(), input); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("error = %v, want unsupported", err)
	}
}

func TestFakePlayerRejectsInvalidAudio(t *testing.T) {
	if err := (&Fake{}).Play(context.Background(), audio.Audio{}); err == nil {
		t.Fatal("Fake.Play accepted invalid audio")
	}
}
