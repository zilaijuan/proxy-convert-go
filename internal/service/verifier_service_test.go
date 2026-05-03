package service

import (
	"reflect"
	"testing"
)

func TestUniqueProxyNameSkipsOccupiedSuffixes(t *testing.T) {
	usedNames := make(map[string]struct{})
	nextSuffix := make(map[string]int)

	got := []string{
		uniqueProxyName("node", usedNames, nextSuffix),
		uniqueProxyName("node_1", usedNames, nextSuffix),
		uniqueProxyName("node", usedNames, nextSuffix),
	}

	want := []string{"node", "node_1", "node_2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected names: got %v want %v", got, want)
	}
}

func TestUniqueProxyNameIncrementsPerBase(t *testing.T) {
	usedNames := make(map[string]struct{})
	nextSuffix := make(map[string]int)

	got := []string{
		uniqueProxyName("node", usedNames, nextSuffix),
		uniqueProxyName("node", usedNames, nextSuffix),
		uniqueProxyName("node", usedNames, nextSuffix),
	}

	want := []string{"node", "node_1", "node_2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected names: got %v want %v", got, want)
	}
}
