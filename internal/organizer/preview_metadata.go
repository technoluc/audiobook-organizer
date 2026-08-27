package organizer

import (
	"os"
)

// PreviewPathWithMetadata plans a move for a known ABS-backed source without
// touching the filesystem beyond statting the source. This is intentionally
// lighter than OrganizePathWithMetadata: dry-run previews should not create
// destination directories or walk/copy source trees just to calculate a plan.
func (o *Organizer) PreviewPathWithMetadata(sourcePath string, metadata Metadata) error {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}

	provider := NewStaticMetadataProvider(metadata)
	prepared, err := o.prepareMetadata(provider)
	if err != nil {
		return err
	}
	if err := prepared.Validate(); err != nil {
		return err
	}

	var targetPath string
	if info.IsDir() {
		targetPath, err = o.layoutCalculator.CalculateTargetPathE(prepared)
	} else {
		targetPath, err = o.calculateSingleFileTargetPathE(sourcePath, prepared)
	}
	if err != nil {
		return err
	}

	o.summary.MetadataFound = append(o.summary.MetadataFound, sourcePath)
	if o.isAlreadyInCorrectLocation(sourcePath, targetPath) {
		return nil
	}
	o.summary.Moves = append(o.summary.Moves, MoveSummary{From: sourcePath, To: targetPath})
	return nil
}
