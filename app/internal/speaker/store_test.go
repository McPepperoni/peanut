package speaker

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"testing"

	"peanut/internal/audio"
	mlspeaker "peanut/internal/ml/speaker"
	"peanut/internal/storage/sqlite"
)

type fakeEmbedder struct {
	embeddings [][]float32
	next       int
}

func (f *fakeEmbedder) Embed(context.Context, audio.Audio) ([]float32, error) {
	result := f.embeddings[f.next]
	f.next++
	return result, nil
}

func TestSpeakerStoreEnrollsAggregatedEmbeddingOnly(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "peanut.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	store := NewSpeakerStore(db)
	sample, err := audio.NewAudio(audio.SampleRate, audio.Channels, make([]float32, 8000))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Enroll(ctx, "alice", []audio.Audio{sample, sample}, &fakeEmbedder{embeddings: [][]float32{{1, 0}, {0, 1}}}); err != nil {
		t.Fatal(err)
	}
	embedding, err := store.Embedding(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	want := float32(1 / math.Sqrt2)
	if len(embedding) != 2 || math.Abs(float64(embedding[0]-want)) > 1e-6 || math.Abs(float64(embedding[1]-want)) > 1e-6 {
		t.Fatalf("embedding = %v, want normalized average", embedding)
	}
	var storedBytes int
	if err := db.QueryRowContext(ctx, `SELECT length(embedding) FROM speakers WHERE id = 'alice'`).Scan(&storedBytes); err != nil {
		t.Fatal(err)
	}
	if storedBytes != 8 {
		t.Fatalf("stored bytes = %d, want embedding only", storedBytes)
	}
}

func TestNameFallsBackToUnknown(t *testing.T) {
	if got := Name(mlspeaker.Result{ID: "alice", Err: errors.New("failed")}); got != "unknown" {
		t.Fatalf("Name = %q, want unknown", got)
	}
}

func TestMatchReturnsEnrolledSpeakerOrUnknown(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "peanut.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	store := NewSpeakerStore(db)
	sample, err := audio.NewAudio(audio.SampleRate, audio.Channels, make([]float32, 320))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Enroll(ctx, "alice", []audio.Audio{sample}, &fakeEmbedder{embeddings: [][]float32{{1, 0}}}); err != nil {
		t.Fatal(err)
	}
	if id, score, err := store.Match(ctx, []float32{1, 0}, 0.8); err != nil || id != "alice" || score < 0.99 {
		t.Fatalf("match = %q, %v, %v", id, score, err)
	}
	if id, _, err := store.Match(ctx, []float32{0, 1}, 0.8); err != nil || id != "" {
		t.Fatalf("unknown match = %q, %v", id, err)
	}
}
