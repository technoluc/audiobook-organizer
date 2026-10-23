// internal/abs/provider.go
// ABS metadata provider for audiobook-organizer

package abs

import (
	"fmt"
	"path"
	"strings"
	"sync"

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

func (p *MetadataProvider) SetClient(client *Client) { p.client = client }

func NewMetadataProvider(apiURL, apiToken, libraryID string, pathMappings []PathMapping) *MetadataProvider {
	return &MetadataProvider{
		client: NewClient(apiURL, apiToken), mapper: NewPathMapper(pathMappings),
		libraryID: libraryID, itemsCache: nil, allLibraries: false,
	}
}

func NewMetadataProviderAllLibraries(apiURL, apiToken string, pathMappings []PathMapping) *MetadataProvider {
	return &MetadataProvider{
		client: NewClient(apiURL, apiToken), mapper: NewPathMapper(pathMappings),
		libraryID: "", itemsCache: nil, allLibraries: true,
	}
}

func NewMetadataProviderWithSQLite(apiURL, apiToken, libraryID, sqlitePath, userInputPath string) (*MetadataProvider, error) {
	mapper, err := NewPathMapperFromSQLite(sqlitePath, userInputPath)
	if err != nil {
		return nil, fmt.Errorf("path discovery failed: %w", err)
	}
	return &MetadataProvider{client: NewClient(apiURL, apiToken), mapper: mapper, libraryID: libraryID}, nil
}

// LoadAllItems fetches all library items from ABS. The list endpoint may omit file
// details, so hydrate compact records through /api/items/:id with bounded concurrency.
func (p *MetadataProvider) LoadAllItems() error {
	if p.allLibraries {
		return p.loadAllLibraries()
	}
	items, err := p.client.GetAllLibraryItems(p.libraryID)
	if err != nil {
		return fmt.Errorf("fetching library items: %w", err)
	}
	p.itemsCache = p.hydrateIncompleteItems(items)
	return nil
}

func (p *MetadataProvider) hydrateIncompleteItems(items []LibraryItem) []LibraryItem {
	const workers = 8
	result := make([]LibraryItem, len(items))
	copy(result, items)
	jobs := make(chan int)
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for index := range jobs {
			item := result[index]
			if !needsItemHydration(&item) || item.ID == "" {
				continue
			}
			detail, err := p.client.GetLibraryItem(item.ID)
			if err != nil || detail == nil {
				continue
			}
			if detail.LibraryID == "" {
				detail.LibraryID = item.LibraryID
			}
			result[index] = *detail
		}
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker()
	}
	for i := range result {
		if needsItemHydration(&result[i]) && result[i].ID != "" {
			jobs <- i
		}
	}
	close(jobs)
	wg.Wait()
	return result
}

func needsItemHydration(item *LibraryItem) bool {
	return len(item.LibraryFiles) == 0 && len(item.Media.AudioFiles) == 0 && item.Media.EbookFile == nil
}

func (p *MetadataProvider) loadAllLibraries() error {
	libraries, err := p.client.GetLibraries()
	if err != nil {
		return fmt.Errorf("fetching libraries: %w", err)
	}
	var allItems []LibraryItem
	for _, lib := range libraries {
		items, err := p.client.GetAllLibraryItems(lib.ID)
		if err != nil {
			continue
		}
		for i := range items {
			items[i].LibraryID = lib.ID
		}
		items = p.hydrateIncompleteItems(items)
		allItems = append(allItems, items...)
	}
	p.itemsCache = allItems
	return nil
}

func (p *MetadataProvider) FindItemByPath(localPath string) (*LibraryItem, error) {
	if p.itemsCache == nil {
		if err := p.LoadAllItems(); err != nil {
			return nil, err
		}
	}
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
			if match != nil && len(normalizeABSPath(item.Path)) == len(normalizeABSPath(match.Path)) {
				return nil, fmt.Errorf("ambiguous ABS items found for path: %s", localPath)
			}
			if match == nil || len(normalizeABSPath(item.Path)) > len(normalizeABSPath(match.Path)) {
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
		if sameABSPath(file.Metadata.Path, absPath) || sameABSPath(file.Metadata.RelPath, absPath) { return true }
	}
	for _, file := range item.Media.AudioFiles {
		if sameABSPath(file.Metadata.Path, absPath) || sameABSPath(file.Metadata.RelPath, absPath) { return true }
	}
	return item.Media.EbookFile != nil && (sameABSPath(item.Media.EbookFile.Metadata.Path, absPath) || sameABSPath(item.Media.EbookFile.Metadata.RelPath, absPath))
}

func sameABSPath(left, right string) bool {
	return left != "" && right != "" && normalizeABSPath(left) == normalizeABSPath(right)
}
func pathContains(parent, candidate string) bool {
	parent, candidate = normalizeABSPath(parent), normalizeABSPath(candidate)
	return parent != "" && (candidate == parent || strings.HasPrefix(candidate, parent+"/"))
}
func normalizeABSPath(value string) string {
	if value == "" { return "" }
	return strings.TrimSuffix(path.Clean(value), "/")
}

func (p *MetadataProvider) FindItemsByLibrary() map[string][]LibraryItem {
	byLib := make(map[string][]LibraryItem)
	for _, item := range p.itemsCache { byLib[item.LibraryID] = append(byLib[item.LibraryID], item) }
	return byLib
}

func (p *MetadataProvider) GetMetadata(localPath string) (organizer.Metadata, error) {
	item, err := p.FindItemByPath(localPath)
	if err != nil { return organizer.NewMetadata(), err }
	metadata := p.convertToOrganizerMetadata(item)
	p.applyFileMetadata(&metadata, item, p.mapper.ToABS(localPath))
	return metadata, nil
}

func (p *MetadataProvider) applyFileMetadata(metadata *organizer.Metadata, item *LibraryItem, absPath string) {
	for _, audioFile := range item.Media.AudioFiles {
		if sameABSPath(audioFile.Metadata.Path, absPath) || sameABSPath(audioFile.Metadata.RelPath, absPath) {
			metadata.TrackNumber = audioFile.TrackNumberFromMeta
			metadata.RawData["track"] = audioFile.TrackNumberFromMeta
			metadata.RawData["disc"] = audioFile.DiscNumberFromMeta
			return
		}
	}
}

// sourcePathForItem returns the path that should actually be moved for an ABS item.
// ABS can assign several library items to the same containing directory (for example
// /audiobooks/Agatha Christie) while each item represents one M4B inside that folder.
// Organizing that shared directory once per item is both incorrect and can make the
// preview appear to hang. When a shared item path has exactly one audio file, use the
// file as the source. A unique book directory remains a directory so sidecars and
// multi-file books keep their existing behaviour.
func (p *MetadataProvider) sourcePathForItem(item *LibraryItem) string {
	absSourcePath := item.Path
	if item.IsFile || absSourcePath == "" || p.isMappedLibraryRoot(absSourcePath) || p.itemPathIsShared(absSourcePath) {
		if filePath := singleAudioFilePath(item); filePath != "" {
			absSourcePath = filePath
		} else if item.IsFile {
			if filePath := preferredAudioFilePath(item); filePath != "" { absSourcePath = filePath }
		}
	}
	return p.mapper.ToLocal(absSourcePath)
}

func (p *MetadataProvider) itemPathIsShared(absPath string) bool {
	if absPath == "" { return false }
	count := 0
	for i := range p.itemsCache {
		if sameABSPath(p.itemsCache[i].Path, absPath) {
			count++
			if count > 1 { return true }
		}
	}
	return false
}

func preferredAudioFilePath(item *LibraryItem) string {
	for _, audioFile := range item.Media.AudioFiles { if audioFile.Metadata.Path != "" { return audioFile.Metadata.Path } }
	for _, libraryFile := range item.LibraryFiles { if libraryFile.Metadata.Path != "" { return libraryFile.Metadata.Path } }
	return ""
}
func singleAudioFilePath(item *LibraryItem) string {
	if len(item.Media.AudioFiles) == 1 { return item.Media.AudioFiles[0].Metadata.Path }
	return ""
}
func (p *MetadataProvider) isMappedLibraryRoot(absPath string) bool {
	for _, mapping := range p.mapper.Mappings { if sameABSPath(mapping.ABSPrefix, absPath) { return true } }
	return false
}

func (p *MetadataProvider) convertToOrganizerMetadata(item *LibraryItem) organizer.Metadata {
	meta := organizer.NewMetadata()
	meta.SourceType = "abs"
	meta.SourcePath = p.sourcePathForItem(item)
	absMedia := item.Media.Metadata
	meta.Title = absMedia.Title
	for _, author := range absMedia.Authors { if author.Name != "" { meta.Authors = append(meta.Authors, author.Name) } }
	if len(meta.Authors) == 0 && absMedia.AuthorName != "" { meta.Authors = append(meta.Authors, absMedia.AuthorName) }
	if len(meta.Authors) == 0 && item.AuthorNamesFirstLast != "" { meta.Authors = append(meta.Authors, splitABSNames(item.AuthorNamesFirstLast)...) }
	if len(meta.Authors) == 0 && item.AuthorNamesLastFirst != "" { meta.Authors = append(meta.Authors, splitABSNames(item.AuthorNamesLastFirst)...) }
	for _, series := range absMedia.Series { if series.Name != "" { meta.Series = append(meta.Series, series.Name) } }
	if len(meta.Series) == 0 && absMedia.SeriesName != "" { meta.Series = append(meta.Series, absMedia.SeriesName) }
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
	meta.RawData["abs_path"] = item.Path
	meta.RawData["abs_relpath"] = item.RelPath
	meta.RawData["abs_duration"] = item.Media.Duration
	meta.RawData["abs_narrator"] = absMedia.NarratorName
	meta.RawData["abs_asin"] = absMedia.ASIN
	meta.RawData["abs_isbn"] = absMedia.ISBN
	meta.RawData["abs_published_year"] = absMedia.PublishedYear
	meta.RawData["abs_explicit"] = absMedia.Explicit
	return meta
}

func (p *MetadataProvider) GetAllItems() ([]organizer.Metadata, error) {
	if p.itemsCache == nil { if err := p.LoadAllItems(); err != nil { return nil, err } }
	results := make([]organizer.Metadata, 0, len(p.itemsCache))
	for i := range p.itemsCache { results = append(results, p.convertToOrganizerMetadata(&p.itemsCache[i])) }
	return results, nil
}

func splitABSNames(value string) []string {
	var names []string
	for _, name := range strings.Split(value, ",") { if name = strings.TrimSpace(name); name != "" { names = append(names, name) } }
	return names
}
func (p *MetadataProvider) ScanLibrary() error { return p.client.ScanLibrary(p.libraryID) }
func (p *MetadataProvider) GetPathMappings() []PathMapping { return p.mapper.Mappings }
func (p *MetadataProvider) Mapper() *PathMapper { return p.mapper }
func (p *MetadataProvider) Client() *Client { return p.client }
