package telegram

import "testing"

func TestSplitReplySegmentsWithImageURL(t *testing.T) {
	segments := splitReplySegments("hello [CQ:image,url=https://example.com/a.png] world")
	if len(segments) != 3 {
		t.Fatalf("segments length = %d, want 3: %#v", len(segments), segments)
	}
	if segments[0].kind != replySegmentText || segments[0].value != "hello" {
		t.Fatalf("first segment = %#v, want hello text", segments[0])
	}
	if segments[1].kind != replySegmentImage || segments[1].value != "https://example.com/a.png" {
		t.Fatalf("second segment = %#v, want image URL", segments[1])
	}
	if segments[2].kind != replySegmentText || segments[2].value != "world" {
		t.Fatalf("third segment = %#v, want world text", segments[2])
	}
}

func TestSplitReplySegmentsWithImageFileAndCQEscapes(t *testing.T) {
	segments := splitReplySegments("[CQ:image,file=https://example.com/a&#44;b.png]")
	if len(segments) != 1 {
		t.Fatalf("segments length = %d, want 1: %#v", len(segments), segments)
	}
	if segments[0].kind != replySegmentImage || segments[0].value != "https://example.com/a,b.png" {
		t.Fatalf("segment = %#v, want decoded image file", segments[0])
	}
}

func TestDecodeDataImage(t *testing.T) {
	data, filename, ok := decodeDataImage("data:image/png;base64,aGVsbG8=")
	if !ok {
		t.Fatalf("decodeDataImage returned ok=false")
	}
	if string(data) != "hello" {
		t.Fatalf("decoded data = %q, want hello", string(data))
	}
	if filename != "image.png" {
		t.Fatalf("filename = %q, want image.png", filename)
	}
}
