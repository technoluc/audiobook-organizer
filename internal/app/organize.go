package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jeeftor/audiobook-organizer/internal/organizer"
)

// OrganizeRequest requests an organization preview or execution.
type OrganizeRequest struct {
	Config OrganizerConfigDTO `json:"config"`
}

const metadataSourceABS = "abs"

// OrganizePreviewResponse contains a dry-run organization summary.
type OrganizePreviewResponse struct {
	Summary organizer.Summary `json:"summary"`
	LogPath string            `json:"log_path,omitempty"`
}

// OrganizeRunResponse contains an executed organization summary.
type OrganizeRunResponse struct {
	Summary organizer.Summary `json:"summary"`
	LogPath string            `json:"log_path,omitempty"`
}

// PreviewOrganize runs the organizer in dry-run mode.
func (s *Service) PreviewOrganize(
	ctx context.Context,
	req OrganizeRequest,
) (*OrganizePreviewResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	org, err := s.executeOrganize(ctx, req, true)
	if err != nil {
		return nil, err
	}
	return &OrganizePreviewResponse{Summary: org.GetSummary()}, nil
}

// RunOrganize runs the organizer with filesystem mutations enabled.
func (s *Service) RunOrganize(
	ctx context.Context,
	req OrganizeRequest,
) (*OrganizeRunResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	org, err := s.executeOrganize(ctx, req, false)
	if err != nil {
		return nil, err
	}
	return &OrganizeRunResponse{Summary: org.GetSummary(), LogPath: org.GetLogPath()}, nil
}

func (s *Service) executeOrganize(ctx context.Context, req OrganizeRequest, dryRun bool) (*organizer.Organizer, error) {
	config := req.Config.ToOrganizerConfig()
	config.DryRun = dryRun
	org, err := organizer.NewOrganizer(&config)
	if err != nil {
		return nil, err
	}
	if req.Config.MetadataSource != metadataSourceABS {
		if err := org.Execute(); err != nil {
			return nil, err
		}
		return org, nil
	}

	if err := org.ResolvePaths(); err != nil {
		return nil, err
	}
	provider, err := s.newABSProviderForInput(req.Config.ABS, org.BaseDir())
	if err != nil {
		return nil, err
	}
	if err := provider.LoadAllItems(); err != nil {
		return nil, fmt.Errorf("loading ABS items: %w", err)
	}
	items, err := provider.GetAllItems()
	if err != nil {
		return nil, fmt.Errorf("getting ABS metadata: %w", err)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].SourcePath < items[j].SourcePath
	})

	processed := 0
	startTime := time.Now()
	seenSourcePaths := make(map[string]struct{})
	for _, item := range items {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		sourcePath := item.SourcePath
		if sourcePath == "" {
			continue
		}
		resolvedSourcePath, err := filepath.EvalSymlinks(sourcePath)
		if err != nil {
			if config.SkipErrors || os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("resolving ABS item path %s: %w", sourcePath, err)
		}
		sourcePath = resolvedSourcePath

		// Never treat the library root itself as one audiobook. A compact ABS
		// record can otherwise make a flat library look like a single giant item.
		if filepath.Clean(sourcePath) == filepath.Clean(org.BaseDir()) {
			continue
		}
		if _, seen := seenSourcePaths[sourcePath]; seen {
			continue
		}
		seenSourcePaths[sourcePath] = struct{}{}

		if !isPathWithin(org.BaseDir(), sourcePath) || !org.IsAllowedSourcePath(sourcePath) {
			continue
		}

		if dryRun {
			err = org.PreviewPathWithMetadata(sourcePath, item)
		} else {
			err = org.OrganizePathWithMetadata(sourcePath, item)
		}
		if err != nil {
			if config.SkipErrors {
				continue
			}
			return nil, fmt.Errorf("organizing ABS item %s: %w", sourcePath, err)
		}
		processed++
	}
	if processed == 0 {
		return nil, fmt.Errorf("no mapped ABS items found under %s", org.BaseDir())
	}

	// Do not call Organizer.Finish for a preview. Finish includes optional source
	// cleanup intended for real runs; in dry-run mode that cleanup can repeatedly
	// rediscover the same empty directories because they are intentionally not
	// deleted. The preview summary is already complete at this point.
	if !dryRun {
		if err := org.Finish(startTime); err != nil {
			return nil, err
		}
	}
	return org, nil
}

func isPathWithin(basePath string, candidatePath string) bool {
	rel, err := filepath.Rel(basePath, candidatePath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
