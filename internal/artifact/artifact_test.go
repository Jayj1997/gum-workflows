package artifact

import "testing"

func TestArtifactRefValidate(t *testing.T) {
	tests := []struct {
		name    string
		ref     ArtifactRef
		wantErr bool
	}{
		{
			name:    "valid ref",
			ref:     ArtifactRef{ID: "a-1", Kind: KindOpenAPI, URI: "file:///tmp/a-1"},
			wantErr: false,
		},
		{
			name:    "empty ID",
			ref:     ArtifactRef{Kind: KindOpenAPI, URI: "file:///tmp/a-1"},
			wantErr: true,
		},
		{
			name:    "empty kind",
			ref:     ArtifactRef{ID: "a-1", URI: "file:///tmp/a-1"},
			wantErr: true,
		},
		{
			name:    "empty URI",
			ref:     ArtifactRef{ID: "a-1", Kind: KindOpenAPI},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ref.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestArtifactValidate(t *testing.T) {
	tests := []struct {
		name    string
		a       Artifact
		wantErr bool
	}{
		{
			name:    "valid artifact",
			a:       Artifact{ID: "a-1", Kind: KindRequirementSpec, Version: "1.0", Data: "spec"},
			wantErr: false,
		},
		{
			name:    "empty ID",
			a:       Artifact{Kind: KindRequirementSpec},
			wantErr: true,
		},
		{
			name:    "empty kind",
			a:       Artifact{ID: "a-1"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.a.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestArtifactRef(t *testing.T) {
	a := Artifact{ID: "a-1", Kind: KindOpenAPI, Version: "1.0", Data: "openapi"}
	ref := a.Ref("file:///tmp/a-1")

	if ref.ID != a.ID || ref.Kind != a.Kind || ref.Version != a.Version {
		t.Fatalf("Ref() = %+v, does not carry artifact identity", ref)
	}
	if ref.URI != "file:///tmp/a-1" {
		t.Fatalf("Ref().URI = %q, want %q", ref.URI, "file:///tmp/a-1")
	}
	if err := ref.Validate(); err != nil {
		t.Fatalf("Ref() produced invalid ref: %v", err)
	}
}
