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
			Metadata: Metadata{Title: "Stil maar", AuthorName: "Carla Kovach"},
			AudioFiles: []AudioFile{{LibraryFile: LibraryFile{Metadata: FileMetadata{
				Path: "/audiobooks/Carla Kovach Stil maar.m4b", RelPath: "Carla Kovach Stil maar.m4b",
			}}}},
		},
	})

	if meta.SourcePath != "/audiobooks/Carla Kovach Stil maar.m4b" {
		t.Fatalf("SourcePath = %q, want flat M4B path", meta.SourcePath)
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
		ID: "flat_002", LibraryID: "lib_main", Path: "/audiobooks", IsFile: false,
		Media: Media{
			Metadata: Metadata{Title: "A Flat Book", AuthorName: "Example Author"},
			AudioFiles: []AudioFile{{LibraryFile: LibraryFile{Metadata: FileMetadata{Path: "/audiobooks/A Flat Book.m4b"}}}},
		},
	})

	if meta.SourcePath != "/books/A Flat Book.m4b" {
		t.Fatalf("SourcePath = %q, want mapped flat M4B path", meta.SourcePath)
	}
}

func TestMetadataProvider_SharedParentDirectoryUsesSingleAudioFile(t *testing.T) {
	provider := NewMetadataProvider(
		"http://example.invalid",
		"test-token",
		"lib_main",
		[]PathMapping{{ABSPrefix: "/audiobooks", LocalPrefix: "/audiobooks"}},
	)
	provider.itemsCache = []LibraryItem{
		{
			ID: "agatha_1", Path: "/audiobooks/Agatha Christie",
			Media: Media{Metadata: Metadata{Title: "Overal is de duivel", AuthorName: "Agatha Christie"}, AudioFiles: []AudioFile{{
				LibraryFile: LibraryFile{Metadata: FileMetadata{Path: "/audiobooks/Agatha Christie/Overal is de duivel.m4b"}},
			}}},
		},
		{
			ID: "agatha_2", Path: "/audiobooks/Agatha Christie",
			Media: Media{Metadata: Metadata{Title: "Moord in de Oriënt-Expres", AuthorName: "Agatha Christie"}, AudioFiles: []AudioFile{{
				LibraryFile: LibraryFile{Metadata: FileMetadata{Path: "/audiobooks/Agatha Christie/Moord in de Oriënt-Expres.m4b"}},
			}}},
		},
	}

	meta := provider.convertToOrganizerMetadata(&provider.itemsCache[0])
	if meta.SourcePath != "/audiobooks/Agatha Christie/Overal is de duivel.m4b" {
		t.Fatalf("SourcePath = %q, want shared-parent M4B path", meta.SourcePath)
	}
}

func TestMetadataProvider_FolderBookKeepsDirectorySourcePath(t *testing.T) {
	provider := NewMetadataProvider(
		"http://example.invalid",
		"test-token",
		"lib_main",
		[]PathMapping{{ABSPrefix: "/audiobooks", LocalPrefix: "/books"}},
	)
	item := LibraryItem{
		ID: "folder_001", LibraryID: "lib_main", Path: "/audiobooks/Neil Gaiman/The Sandman", IsFile: false,
		Media: Media{
			Metadata: Metadata{Title: "The Sandman", AuthorName: "Neil Gaiman"},
			AudioFiles: []AudioFile{{LibraryFile: LibraryFile{Metadata: FileMetadata{Path: "/audiobooks/Neil Gaiman/The Sandman/01.m4b"}}}},
		},
	}
	provider.itemsCache = []LibraryItem{item}

	meta := provider.convertToOrganizerMetadata(&provider.itemsCache[0])
	if meta.SourcePath != "/books/Neil Gaiman/The Sandman" {
		t.Fatalf("SourcePath = %q, want folder path", meta.SourcePath)
	}
}
