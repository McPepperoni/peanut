package speaker

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"peanut/internal/audio"
	mlspeaker "peanut/internal/ml/speaker"
	"peanut/internal/storage/sqlite"
)

type Embedder interface {
	Embed(context.Context, audio.Audio) ([]float32, error)
}

type Store struct{ db *sqlite.DB }

func NewSpeakerStore(db *sqlite.DB) *Store { return &Store{db: db} }

func (s *Store) Enroll(ctx context.Context, id string, samples []audio.Audio, embedder Embedder) error {
	if s == nil || s.db == nil || id == "" || len(samples) == 0 || embedder == nil {
		return errors.New("speaker store, ID, samples, and embedder are required")
	}
	var total []float32
	for _, sample := range samples {
		if err := sample.Validate(); err != nil {
			return err
		}
		embedding, err := embedder.Embed(ctx, sample)
		if err != nil {
			return fmt.Errorf("embed enrollment sample: %w", err)
		}
		if len(embedding) == 0 || total != nil && len(embedding) != len(total) {
			return errors.New("speaker embeddings must have a consistent non-zero size")
		}
		if total == nil {
			total = make([]float32, len(embedding))
		}
		for index, value := range embedding {
			total[index] += value
		}
	}
	var length float64
	for _, value := range total {
		length += float64(value * value)
	}
	if length == 0 || math.IsNaN(length) || math.IsInf(length, 0) {
		return errors.New("speaker embedding must have finite non-zero magnitude")
	}
	denominator := float32(math.Sqrt(length))
	encoded := make([]byte, len(total)*4)
	for index, value := range total {
		binary.LittleEndian.PutUint32(encoded[index*4:], math.Float32bits(value/denominator))
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO speakers (id, embedding) VALUES (?, ?) ON CONFLICT(id) DO UPDATE SET embedding = excluded.embedding`, id, encoded)
	if err != nil {
		return fmt.Errorf("store speaker embedding: %w", err)
	}
	return nil
}

func (s *Store) Embedding(ctx context.Context, id string) ([]float32, error) {
	if s == nil || s.db == nil || id == "" {
		return nil, errors.New("speaker store and ID are required")
	}
	var encoded []byte
	if err := s.db.QueryRowContext(ctx, `SELECT embedding FROM speakers WHERE id = ?`, id).Scan(&encoded); err != nil {
		return nil, fmt.Errorf("load speaker embedding: %w", err)
	}
	if len(encoded) == 0 || len(encoded)%4 != 0 {
		return nil, errors.New("invalid stored speaker embedding")
	}
	embedding := make([]float32, len(encoded)/4)
	for index := range embedding {
		embedding[index] = math.Float32frombits(binary.LittleEndian.Uint32(encoded[index*4:]))
	}
	return embedding, nil
}

func Name(result mlspeaker.Result) string {
	if result.Err != nil || result.ID == "" {
		return "unknown"
	}
	return result.ID
}
