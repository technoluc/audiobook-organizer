// internal/abs/provider.go
// ABS metadata provider for audiobook-organizer

package abs

import (
	"fmt"
	"path"
	"strings"

	"github.com/jeeftor/audiobook-organizer/internal/organizer"
)

// MetadataProvider provides metadata from ABS
type MetadataProvider struct {
	client       *Client
	mapper       *PathMapper
	libraryID    string
	itemsCache   []LibraryItem // Cache of all items for path lookup
	allLibraries bool          // If true, scan all libraries not just one
}

// SetClient allows replacing the client (useful for adding custom headers)
func (p *MetadataProvider) SetClient(client *Client) {
	p.client = client
}

// NewMetadataProvider creates an ABS metadata provider
// Supports both API-only (with manual mappings) and SQLite+API modes
func NewMetadataProvider(
	apiURL, apiToken, libraryID string,
	pathMappings []PathMapping,
) *MetadataProvider {
	client := NewClient(apiURL, apiToken)
	mapper := NewPathMapper(pathMappings)

	return &MetadataProvider{
		client:       client,
		mapper:       mapper,
		libraryID:    libraryID,
		itemsCache:   nil,
		allLibraries: false,
	}
}

// NewMetadataProviderAllLibraries creates provider that scans ALL libraries
func NewMetadataProviderAllLibraries(
	apiURL, apiToken string,
	pathMappings []PathMapping,
) *MetadataProvider {
	client := NewClient(apiURL, apiToken)
	mapper := NewPathMapper(pathMappings)

	return &MetadataProvider{
		client:       client,
		mapper:       mapper,
		libraryID:    "", // Empty = all libraries
		itemsCache:   nil,
		allLibraries: true,
	}
}

// NewMetadataProviderWithSQLite creates provider with auto path discovery from SQLite
func NewMetadataProviderWithSQLite(
	apiURL, apiToken, libraryID, sqlitePath, userInputPath string,
) (*MetadataProvider, error) {
	// Discover path mappings from SQLite
	mapper, err := NewPathMapperFromSQLite(sqlitePath, userInputPath)
	if err != nil {
		return nil, fmt.Errorf("path discovery failed: %w", err)
	}

	client := NewClient(apiURL, apiToken)

	return &MetadataProvider{
		client:     client,
		mapper:     mapper,
		libraryID:  libraryID,
		itemsCache: nil,
	}, nil
}

// LoadAllItems fetches all library items from ABS (for path matching)
func (p *MetadataProvider) LoadAllItems() error {
	if p.allLibraries {
		return p.loadAllLibraries()
	}

	items, err := p.client.GetAllLibraryItems(p.libraryID)
	if err != nil {
		return fmt.Errorf("fetching library items: %w", err)
	}
	p.itemsCache = items
	return nil
}

// loadAllLibraries fetches items from ALL libraries
func (p *MetadataProvider) loadAllLibraries() error {
	libraries, err := p.client.GetLibraries()
	if err != nil {
		return fmt.Errorf("fetching libraries: %w", err)
	}

	var allItems []LibraryItem
	for _, lib := range libraries {
		items, err := p.client.GetAllLibraryItems(lib.ID)
		if err != nil {
			// Log warning but continue with other libraries
			continue
		}
		// Tag each item with its library info for later
		for i := range items {
			items[i].LibraryID = lib.ID
		}
		allItems = append(allItems, items...)
	}

	p.itemsCache = allItems
	return nil
}

// FindItemByPath finds an ABS item matching a local ABS item directory or file.
// Exact file records take precedence over a containing item directory.
// Works across all libraries if allLibraries mode is enabled.
func (p *MetadataProvider) FindItemByPath(localPath string) (*LibraryItem, error) {
	if p.itemsCache == nil {
		if err := p.LoadAllItems(); err != nil {
			return nil, err
		}
	}

	// Convert local path to ABS path
	absPath := p.mapper.ToABS(localPath)

	for i := range p.itemsCache {
		if itemContainsFile(&p.itemsCache[i], absPath) {
			return &p.itemsCache[i], nil
		}
	}

	var match *LibraryItem
	for i := range p.itemsCache {
		item := &p.itemsCache[i]
		if sameABSPath(item.Path, absPath) || sameABSPath(item.RelPath, absPath) {
			return item, nil
		}
		if pathContains(item.Path, absPath) {
			if match != nil &&
				len(normalizeABSPath(item.Path)) == len(normalizeABSPath(match.Path)) {
				return nil, fmt.Errorf("ambiguous ABS items found for path: %s", localPath)
			}
			if match == nil ||
				len(normalizeABSPath(item.Path)) > len(normalizeABSPath(match.Path)) {
				match = item
			}
		}
	}
	if match != nil {
		return match, nil
	}

	return nil, fmt.Errorf("no ABS item found for path: %s", localPath)
}

func itemContainsFile(item *LibraryItem, absPath string) bool {
	for _, file := range item.LibraryFiles {
		if sameABSPath(file.Metadata.Path, absPath) || sameABSPath(file.Metadata.RelPath, absPath) {
			return true
		}
	}
	for _, file := range item.Media.AudioFiles {
		if sameABSPath(file.Metadata.Path, absPath) || sameABSPath(file.Metadata.RelPath, absPath) {
			return true
		}
	}
	return item.Media.EbookFile != nil &&
		(sameABSPath(item.Media.EbookFile.Metadata.Path, absPath) ||
			sameABSPath(item.Media.EbookFile.Metadata.RelPath, absPath))
}

func sameABSPath(left, right string) bool {
	return left != "" && right != "" && normalizeABSPath(left) == normalizeABSPath(right)
}

func pathContains(parent, candidate string) bool {
	parent = normalizeABSPath(parent)
	candidate = normalizeABSPath(candidate)
	return parent != "" && (candidate == parent || strings.HasPrefix(candidate, parent+"/"))
}

func normalizeABSPath(value string) string {
	if value == "" {
		return ""
	}
	return strings.TrimSuffix(path.Clean(value), "/")
}

// FindItemsByLibrary returns items grouped by library ID
func (p *MetadataProvider) FindItemsByLibrary() map[string][]LibraryItem {
	byLib := make(map[string][]LibraryItem)
	for _, item := range p.itemsCache {
		byLib[item.LibraryID] = append(byLib[item.LibraryID], item)
	}
	return byLib
}

// GetMetadata returns metadata for a local audiobook path
// This implements the organizer.MetadataProvider interface
func (p *MetadataProvider) GetMetadata(localPath string) (organizer.Metadata, error) {
	// Find the ABS item for this path
	item, err := p.FindItemByPath(localPath)
	if err != nil {
		return organizer.NewMetadata(), err
	}

	metadata := p.convertToOrganizerMetadata(item)
	p.applyFileMetadata(&metadata, item, p.mapper.ToABS(localPath))
	return metadata, nil
}

func (p *MetadataProvider) applyFileMetadata(
	metadata *organizer.Metadata,
	item *LibraryItem,
	absPath string,
) {
	for _, audioFile := range item.Media.AudioFiles {
		if sameABSPath(audioFile.Metadata.Path, absPath) ||
			sameABSPath(audioFile.Metadata.RelPath, absPath) {
			metadata.TrackNumber = audioFile.TrackNumberFromMeta
			metadata.RawData["track"] = audioFile.TrackNumberFromMeta
			metadata.RawData["disc"] = audioFile.DiscNumberFromMeta
			return
		}
	}
}

// sourcePathForItem returns the path that should actually be moved for an ABS item.
//
// ABS stores its metadata in its own database/appdata and exposes that metadata via
// the API. The audiobook files therefore do not need metadata.json sidecars beside
// them. For normal folder-based books item.Path is the book directory. For a flat
// library, however, ABS can represent the item at the library root while the actual
// audiobook path lives in media.audioFiles[].metadata.path. In that case using
// item.Path makes every flat book appear to be the same directory and only a small
// subset can be organized correctly.
func (p *MetadataProvider) sourcePathForItem(item *LibraryItem) string {
	absSourcePath := item.Path

	// A file-backed ABS item should always resolve to its actual media file when ABS
	// provides one. This is the common representation for a single M4B in a flat
	// Audiobookshelf library.
	if item.IsFile {
		if filePath := preferredAudioFilePath(item); filePath != "" {
			absSourcePath = filePath
		}
	}

	// Some ABS versions/scanners expose a flat item with item.Path equal to the
	// library root instead of marking IsFile. If exactly one audio file belongs to
	// the item, that file is the unambiguous source to organize.
	if absSourcePath == "" || p.isMappedLibraryRoot(absSourcePath) {
		if filePath := singleAudioFilePath(item); filePath != "" {
			absSourcePath = filePath
		}
	}

	return p.mapper.ToLocal(absSourcePath)
}

func preferredAudioFilePath(item *LibraryItem) string {
	for _, audioFile := range item.Media.AudioFiles {
		if audioFile.Metadata.Path != "" {
			return audioFile.Metadata.Path
		}
	}
	for _, libraryFile := range item.LibraryFiles {
		if libraryFile.Metadata.Path != "" {
			return libraryFile.Metadata.Path
		}
	}
	return ""
}

func singleAudioFilePath(item *LibraryItem) string {
	if len(item.Media.AudioFiles) == 1 {
		return item.Media.AudioFiles[0].Metadata.Path
	}
	return ""
}

func (p *MetadataProvider) isMappedLibraryRoot(absPath string) bool {
	for _, mapping := range p.mapper.Mappings {
		if sameABSPath(mapping.ABSPrefix, absPath) {
			return true
		}
	}
	return false
}

// convertToOrganizerMetadata converts ABS LibraryItem to organizer.Metadata
func (p *MetadataProvider) convertToOrganizerMetadata(item *LibraryItem) organizer.Metadata {
	meta := organizer.NewMetadata()
	meta.SourceType = "abs"
	meta.SourcePath = p.sourcePathForItem(item)

	absMedia := item.Media.Metadata

	// Title
	meta.Title = absMedia.Title

	// Authors - handle both array format and flattened string format
	for _, author := range absMedia.Authors {
		if author.Name != "" {
			meta.Authors = append(meta.Authors, author.Name)
		}
	}
	// Also check flattened authorName if array is empty
	if len(meta.Authors) == 0 && absMedia.AuthorName != "" {
		meta.Authors = append(meta.Authors, absMedia.AuthorName)
	}
	if len(meta.Authors) == 0 && item.AuthorNamesFirstLast != "" {
		meta.Authors = append(meta.Authors, splitABSNames(item.AuthorNamesFirstLast)...)
	}
	if len(meta.Authors) == 0 && item.AuthorNamesLastFirst != "" {
		meta.Authors = append(meta.Authors, splitABSNames(item.AuthorNamesLastFirst)...)
	}

	// Series
	for _, series := range absMedia.Series {
		if series.Name != "" {
			meta.Series = append(meta.Series, series.Name)
		}
	}
	if len(meta.Series) == 0 && absMedia.SeriesName != "" {
		meta.Series = append(meta.Series, absMedia.SeriesName)
	}

	// Store ABS-specific data in RawData for advanced use
	meta.RawData["title"] = meta.Title
	meta.RawData["authors"] = strings.Join(meta.Authors, ", ")
	meta.RawData["series"] = strings.Join(meta.Series, ", ")
	meta.RawData["series_number"] = absMedia.SeriesSequence
	meta.RawData["narrator"] = absMedia.NarratorName
	meta.RawData["publisher"] = absMedia.Publisher
	meta.RawData["published_year"] = absMedia.PublishedYear
	meta.RawData["published_date"] = absMedia.PublishedDate
	meta.RawData["language"] = absMedia.Language
	meta.RawData["genres"] = strings.Join(absMedia.Genres, ", ")
	meta.RawData["tags"] = strings.Join(absMedia.Tags, ", ")
	meta.RawData["source_path"] = meta.SourcePath
	meta.RawData["authorNamesFirstLast"] = item.AuthorNamesFirstLast
	meta.RawData["authorNamesLastFirst"] = item.AuthorNamesLastFirst
	meta.RawData["abs_item_id"] = item.ID
	meta.RawData["abs_library_id"] = item.LibraryID
	meta.RawData["abs_path"] = item.Path // Original ABS item path before source-file resolution/mapping
	meta.RawData["abs_relpath"] = item.RelPath
	meta.RawData["abs_duration"] = item.Media.Duration
	meta.RawData["abs_narrator"] = absMedia.NarratorName
	meta.RawData["abs_asin"] = absMedia.ASIN
	meta.RawData["abs_isbn"] = absMedia.ISBN
	meta.RawData["abs_published_year"] = absMedia.PublishedYear
	meta.RawData["abs_explicit"] = absMedia.Explicit

	return meta
}

// GetAllItems returns all library items as organizer metadata
func (p *MetadataProvider) GetAllItems() ([]organizer.Metadata, error) {
	if p.itemsCache == nil {
		if err := p.LoadAllItems(); err != nil {
			return nil, err
		}
	}

	var results []organizer.Metadata
	for _, item := range p.itemsCache {
		meta := p.convertToOrganizerMetadata(&item)
		results = append(results, meta)
	}

	return results, nil
}

func splitABSNames(value string) []string {
	var names []string
	for _, name := range strings.Split(value, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// ScanLibrary triggers an ABS library scan
func (p *MetadataProvider) ScanLibrary() error {
	return p.client.ScanLibrary(p.libraryID)
}

// GetPathMappings returns the current path mappings (for display/debugging)
func (p *MetadataProvider) GetPathMappings() []PathMapping {
	return p.mapper.Mappings
}

// Mapper returns the path mapper (for path conversion)
func (p *MetadataProvider) Mapper() *PathMapper {
	return p.mapper
}

// Client returns the underlying ABS API client (for advanced use)
func (p *MetadataProvider) Client() *Client {
	return p.client
}
