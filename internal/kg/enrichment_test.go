package kg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractStringValue(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  string
	}{
		{
			name:  "plain string",
			input: "Hello, World!",
			want:  "Hello, World!",
		},
		{
			name:  "localized JSON-LD object with @value",
			input: map[string]interface{}{"@value": "Massey Hall", "@language": "en"},
			want:  "Massey Hall",
		},
		{
			name:  "object with @value key only",
			input: map[string]interface{}{"@value": "some description"},
			want:  "some description",
		},
		{
			name:  "object missing @value key",
			input: map[string]interface{}{"@id": "http://example.org/123"},
			want:  "",
		},
		{
			name:  "nil value",
			input: nil,
			want:  "",
		},
		{
			name:  "integer value",
			input: 42,
			want:  "",
		},
		{
			name:  "float value",
			input: 3.14,
			want:  "",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractStringValue(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestInferAuthorityCode_Exported(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		wantCode string
	}{
		{
			name:     "Wikidata",
			uri:      "http://www.wikidata.org/entity/Q1234567",
			wantCode: "wikidata",
		},
		{
			name:     "MusicBrainz",
			uri:      "https://musicbrainz.org/artist/abc-123",
			wantCode: "musicbrainz",
		},
		{
			name:     "ISNI",
			uri:      "https://isni.org/isni/0000000123456789",
			wantCode: "isni",
		},
		{
			name:     "OpenStreetMap",
			uri:      "https://www.openstreetmap.org/way/12345",
			wantCode: "osm",
		},
		{
			name:     "Artsdata",
			uri:      "http://kg.artsdata.ca/resource/K11-211",
			wantCode: "artsdata",
		},
		{
			name:     "unknown authority returns empty string",
			uri:      "https://example.com/entity/123",
			wantCode: "",
		},
		{
			name:     "empty URI returns empty string",
			uri:      "",
			wantCode: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := InferAuthorityCode(tt.uri)
			assert.Equal(t, tt.wantCode, code)
		})
	}
}
