package abs

import "testing"

func TestMetadataProvider_FlatFileUsesAudioFilePath(t *testing.T) {
	provider := NewMetadataProvider(
		"http://example.invalid",
		"test-token",
		"lib_main",
		[]PathMapping{{ABSPrefix: "/audiobooks", LocalPrefix: "/audiobooks"}},
	)

	meta := provider.convertToOrganizerMetadata(&LibraryItem{
		ID:        "flat_001",
		LibraryID: "lib_main",
		Path:      "/audiobooks",
		RelPath:   "",
		IsFile:    true,
		Media: Media{
			Metadata: Metadata{
				Title:      "Stil maar",
				AuthorName: "Carla Kovach",
			},
			AudioFiles: []AudioFile{{
				LibraryFile: LibraryFile{Metadata: FileMetadata{
					Path:    "/audiobooks/Carla Kovach Stil maar.m4b",
					RelPath: "Carla Kovach Stil maar.m4b",
				}},
			}},
		},
	})

	if meta.SourcePath != "/audiobooks/Carla Kovach Stil maar.m4b" {
		t.Fatalf("SourcePath = %q, want flat M4B path", meta.SourcePath)
	}
	if meta.Title != "Stil maar" {
		t.Fatalf("Title = %q, want Stil maar", meta.Title)
	}
	if len(meta.Authors) != 1 || meta.Authors[0] != "Carla Kovach" {
		t.Fatalf("Authors = %v, want [Carla Kovach]", meta.Authors)
	}
}

func TestMetadataProvider_LibraryRootWithSingleAudioFileUsesFilePath(t *testing.T) {
	provider := NewMetadataProvider(
		"http://example.invalid",
		"test-token",
		"lib_main",
		[]PathMapping{{ABSPrefix: "/audiobooks", LocalPrefix: "/books"}},
	)

	meta := provider.convertToOrganizerMetadata(&LibraryItem{
		ID:        "flat_002",
		LibraryID: "lib_main",
		Path:      "/audiobooks",
		IsFile:    false,
		Media: Media{
			Metadata: Metadata{Title: "A Flat Book", AuthorName: "Example Author"},
			AudioFiles: []AudioFile{{
				LibraryFile: LibraryFile{Metadata: FileMetadata{
					Path: "/audiobooks/A Flat Book.m4b",
				}},
			}},
		},
	})

	if meta.SourcePath != "/books/A Flat Book.m4b" {
		t.Fatalf("SourcePath = %q, want mapped flat M4B path", meta.SourcePath)
	}
}

func TestMetadataProvider_FolderBookKeepsDirectorySourcePath(t *testing.T) {
	provider := NewMetadataProvider(
		"http://example.invalid",
		"test-token",
		"lib_main",
		[]PathMapping{{ABSPrefix: "/audiobooks", LocalPrefix: "/books"}},
	)

	meta := provider.convertToOrganizerMetadata(&LibraryItem{
		ID:        "folder_001",
		LibraryID: "lib_main",
		Path:      "/audiobooks/Neil Gaiman/The Sandman",
		IsFile:    false,
		Media: Media{
			Metadata: Metadata{Title: "The Sandman", AuthorName: "Neil Gaiman"},
			AudioFiles: []AudioFile{{
				LibraryFile: LibraryFile{Metadata: FileMetadata{
					Path: "/audiobooks/Neil Gaiman/The Sandman/01.m4b",
				}},
			}},
		},
	})

	if meta.SourcePath != "/books/Neil Gaiman/The Sandman" {
		t.Fatalf("SourcePath = %q, want folder path", meta.SourcePath)
	}
}
