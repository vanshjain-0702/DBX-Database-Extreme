package engine

import (
	"math"
	"testing"
)

func TestValidateSpaceName(t *testing.T) {
	if err := ValidateSpaceName("text"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSpaceName("clip-v1.2"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSpaceName(""); err == nil {
		t.Fatal("empty space")
	}
	if err := ValidateSpaceName("has space"); err == nil {
		t.Fatal("whitespace space")
	}
	if VectorSpaceKey("mem", "") != "mem" {
		t.Fatal("default space must keep the index key")
	}
	if VectorSpaceKey("mem", "text") == "mem" || VectorSpaceKey("mem", "text") == VectorSpaceKey("mem", "image") {
		t.Fatal("named spaces must be distinct keys")
	}
}

func TestVSearchMinScoreAndExclude(t *testing.T) {
	store := NewVectorStore(New(16), t.TempDir(), 0)
	defer store.CloseAll()
	if err := store.VAdd("idx", "a", []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	if err := store.VAdd("idx", "b", []float32{0, 1}); err != nil {
		t.Fatal(err)
	}
	hits, err := store.VSearchOpts("idx", []float32{1, 0}, SearchOpts{K: 5, HasMinScore: true, MinScore: 0.9})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != "a" {
		t.Fatalf("min_score hits = %#v", hits)
	}

	got, err := store.GetVector("idx", "a")
	if err != nil || len(got) != 2 {
		t.Fatalf("GetVector = %v %v", got, err)
	}
	similar, err := store.VSearchOpts("idx", got, SearchOpts{K: 5, ExcludeID: "a"})
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range similar {
		if hit.ID == "a" {
			t.Fatal("VSIM must not return the query id")
		}
	}
	if len(similar) == 0 || similar[0].ID != "b" {
		t.Fatalf("similar = %#v", similar)
	}
}

func TestNamedSpacesAreIsolated(t *testing.T) {
	store := NewVectorStore(New(16), t.TempDir(), 0)
	defer store.CloseAll()
	text := VectorSpaceKey("mem", "text")
	image := VectorSpaceKey("mem", "image")
	if err := store.VAdd(text, "doc1", []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	if err := store.VAdd(image, "doc1", []float32{0, 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.VAdd("mem", "doc1", []float32{0.7, 0.7}); err != nil {
		t.Fatal(err)
	}
	textHits, err := store.VSearch(text, []float32{1, 0}, 3, nil)
	if err != nil || len(textHits) != 1 || textHits[0].ID != "doc1" {
		t.Fatalf("text space = %#v %v", textHits, err)
	}
	if math.Abs(float64(textHits[0].Score-1)) > 0.05 {
		t.Fatalf("text cosine = %f", textHits[0].Score)
	}
	defaultHits, err := store.VSearch("mem", []float32{1, 0}, 3, nil)
	if err != nil || len(defaultHits) != 1 {
		t.Fatalf("default space leaked named rows: %#v %v", defaultHits, err)
	}
}

func TestVFuseRanksAgreementAcrossSpaces(t *testing.T) {
	store := NewVectorStore(New(16), t.TempDir(), 0)
	defer store.CloseAll()
	text := VectorSpaceKey("mem", "text")
	image := VectorSpaceKey("mem", "image")
	rows := []struct {
		id    string
		text  []float32
		image []float32
	}{
		{"agree", []float32{1, 0}, []float32{1, 0}},
		{"split", []float32{1, 0}, []float32{0, 1}},
		{"other", []float32{0, 1}, []float32{0, 1}},
	}
	for _, row := range rows {
		if err := store.VAdd(text, row.id, row.text); err != nil {
			t.Fatal(err)
		}
		if err := store.VAdd(image, row.id, row.image); err != nil {
			t.Fatal(err)
		}
	}
	hits, err := store.VFuse("mem", []SpaceQuery{
		{Space: "text", Query: []float32{1, 0}},
		{Space: "image", Query: []float32{1, 0}},
	}, []float32{1, 1}, SearchOpts{K: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) < 1 || hits[0].ID != "agree" {
		t.Fatalf("fused ranking = %#v", hits)
	}
	if hits[0].Score <= hits[1].Score {
		t.Fatalf("agreement should outrank a split: %#v", hits)
	}
}
