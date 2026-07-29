package core

import "testing"

func TestEncodeBucketValueKeepsIntegerFloatsAsIntegers(t *testing.T) {
	if got := encodeBucketValue(float64(8080)); got != "d:8080" {
		t.Fatalf("encodeBucketValue(float64(8080)) = %q, want d:8080", got)
	}
	if got := encodeBucketValue(1.5); got != "f:1.500000" {
		t.Fatalf("encodeBucketValue(1.5) = %q, want f:1.500000", got)
	}
}
