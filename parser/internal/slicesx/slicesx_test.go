package slicesx

import (
	"reflect"
	"testing"
)

func TestUnique(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		want []int
	}{
		{name: "empty", in: nil, want: nil},
		{name: "no dupes", in: []int{1, 2, 3}, want: []int{1, 2, 3}},
		{name: "dupes", in: []int{3, 1, 2, 1, 3}, want: []int{3, 1, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Unique(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Unique(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
