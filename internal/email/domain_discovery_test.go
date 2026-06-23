package email

import (
	"reflect"
	"testing"
)

func TestAppendUniqueDomainsNormalizesFiltersAndKeepsOrder(t *testing.T) {
	got := appendUniqueDomains([]string{"Existing.COM"}, " @Dynamic-Mail.com ", "bad/path.com", "dynamic-mail.com", "two.example.org")
	want := []string{"Existing.COM", "dynamic-mail.com", "two.example.org"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("domains=%v, want %v", got, want)
	}
}
