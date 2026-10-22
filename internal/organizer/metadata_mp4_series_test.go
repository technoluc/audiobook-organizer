package organizer

import "testing"

func TestGetSeriesFromRawTags(t *testing.T) {
	series, index := getSeriesFromRawTags(map[string]interface{}{
		"series":      "\x00\x00\x00\x00Het Lied van IJs en Vuur ",
		"series-part": "\x00\x00\x00\x002",
		"trkn":        7,
	})

	if series != "Het Lied van IJs en Vuur" {
		t.Fatalf("series = %q", series)
	}
	if index != 2 {
		t.Fatalf("series index = %v", index)
	}
}

func TestGetSeriesFromRawTagsSupportsSeriesIndexAlias(t *testing.T) {
	series, index := getSeriesFromRawTags(map[string]interface{}{
		"SERIES":       "Discworld",
		"SERIES_INDEX": "3.5",
	})

	if series != "Discworld" || index != 3.5 {
		t.Fatalf("series = %q, index = %v", series, index)
	}
}
