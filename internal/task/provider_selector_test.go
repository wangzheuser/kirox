package task

import (
	"sort"
	"sync"
	"testing"
)

func TestEmailProviderSelectorRoundRobinsSerially(t *testing.T) {
	selector := newEmailProviderSelector([]string{"emailnator", "mailgw", "dropmail"})

	got := make([]string, 0, 7)
	for i := 0; i < 7; i++ {
		got = append(got, selector.Next())
	}

	want := []string{"emailnator", "mailgw", "dropmail", "emailnator", "mailgw", "dropmail", "emailnator"}
	if len(got) != len(want) {
		t.Fatalf("len=%d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("round robin mismatch at %d: got %#v, want %#v", i, got, want)
		}
	}
}

func TestEmailProviderSelectorRoundRobinsConcurrently(t *testing.T) {
	selector := newEmailProviderSelector([]string{"emailnator", "mailgw", "dropmail"})

	const total = 300
	results := make([]string, total)
	var wg sync.WaitGroup
	wg.Add(total)
	for i := 0; i < total; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = selector.Next()
		}(i)
	}
	wg.Wait()

	counts := map[string]int{}
	for _, provider := range results {
		counts[provider]++
	}
	if counts["emailnator"] != 100 || counts["mailgw"] != 100 || counts["dropmail"] != 100 {
		t.Fatalf("concurrent round robin counts mismatch: %#v", counts)
	}
}

func TestNormalizeStartEmailProvidersDeduplicatesAndDefaults(t *testing.T) {
	got, err := normalizeStartEmailProviders([]string{" emailnator ", "mailgw", "emailnator", ""})
	if err != nil {
		t.Fatalf("normalizeStartEmailProviders returned error: %v", err)
	}
	want := []string{"emailnator", "mailgw"}
	if len(got) != len(want) {
		t.Fatalf("len=%d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("providers mismatch: got %#v, want %#v", got, want)
		}
	}

	defaulted, err := normalizeStartEmailProviders(nil)
	if err != nil {
		t.Fatalf("default normalize returned error: %v", err)
	}
	if len(defaulted) != 1 || defaulted[0] != "outlook" {
		t.Fatalf("empty provider list should default to outlook, got %#v", defaulted)
	}
}

func TestNormalizeStartEmailProvidersRejectsInvalidProvider(t *testing.T) {
	if _, err := normalizeStartEmailProviders([]string{"emailnator", "invalid"}); err == nil {
		t.Fatalf("invalid provider should be rejected")
	}
}

func TestEmailProviderSelectorSetIsImmutable(t *testing.T) {
	providers := []string{"emailnator", "mailgw"}
	selector := newEmailProviderSelector(providers)
	providers[0] = "dropmail"

	got := []string{selector.Next(), selector.Next(), selector.Next()}
	sort.Strings(got[:2])
	if got[0] != "emailnator" || got[1] != "mailgw" || got[2] != "emailnator" {
		t.Fatalf("selector should keep an internal copy, got %#v", got)
	}
}
