package gtfs

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strings"
)

// ParseZip extracts and parses all GTFS CSV files from a zip archive.
// StopTimes are NOT loaded into memory here — they are streamed during import.
func ParseZip(path string, logger *slog.Logger) (*Feed, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	feed := &Feed{}

	for _, f := range r.File {
		switch f.Name {
		case "agency.txt":
			feed.Agencies, err = parseCSVFile[Agency](f)
		case "routes.txt":
			feed.Routes, err = parseCSVFile[Route](f)
		case "stops.txt":
			feed.Stops, err = parseCSVFile[Stop](f)
		case "trips.txt":
			feed.Trips, err = parseCSVFile[Trip](f)
		case "calendar.txt":
			feed.Calendar, err = parseCSVFile[CalendarEntry](f)
		case "calendar_dates.txt":
			feed.CalendarDates, err = parseCSVFile[CalendarDate](f)
		// stop_times.txt and shapes.txt are streamed during import, not loaded here
		}
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", f.Name, err)
		}
	}

	logger.Info("GTFS feed parsed",
		"agencies", len(feed.Agencies),
		"routes", len(feed.Routes),
		"stops", len(feed.Stops),
		"trips", len(feed.Trips),
		"calendar", len(feed.Calendar),
		"calendar_dates", len(feed.CalendarDates),
	)

	return feed, nil
}

// parseCSVFile reads a single CSV file from the zip and decodes it into a slice of T.
func parseCSVFile[T any](f *zip.File) ([]T, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer rc.Close()

	reader := csv.NewReader(rc)
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	// Strip BOM from first field if present
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\xef\xbb\xbf")
	}

	// Build column-to-field index
	fieldMap := buildFieldMap[T](header)

	var results []T
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read record: %w", err)
		}
		item := decodeRecord[T](record, fieldMap)
		results = append(results, item)
	}

	return results, nil
}

// StreamCSVFile opens a CSV file from a zip and returns a function that yields one record at a time.
// Used for large files like stop_times.txt and shapes.txt to avoid loading them into memory.
type CSVStreamer struct {
	rc       io.ReadCloser
	reader   *csv.Reader
	fieldMap []fieldMapping
}

type fieldMapping struct {
	csvIndex   int
	fieldIndex int
}

// OpenCSVStream opens a CSV file from the zip for streaming.
func OpenCSVStream[T any](f *zip.File) (*CSVStreamer, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}

	reader := csv.NewReader(rc)
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		rc.Close()
		return nil, fmt.Errorf("read header: %w", err)
	}

	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\xef\xbb\xbf")
	}

	return &CSVStreamer{
		rc:       rc,
		reader:   reader,
		fieldMap: buildFieldMap[T](header),
	}, nil
}

// Next reads the next record. Returns io.EOF when done.
func (s *CSVStreamer) Next(out any) error {
	record, err := s.reader.Read()
	if err != nil {
		return err
	}
	v := reflect.ValueOf(out).Elem()
	for _, fm := range s.fieldMap {
		if fm.csvIndex < len(record) {
			v.Field(fm.fieldIndex).SetString(record[fm.csvIndex])
		}
	}
	return nil
}

// Close releases the underlying reader.
func (s *CSVStreamer) Close() error {
	return s.rc.Close()
}

// buildFieldMap creates a mapping from CSV column positions to struct field positions.
func buildFieldMap[T any](header []string) []fieldMapping {
	var t T
	typ := reflect.TypeOf(t)

	// Build a map of csv tag -> field index
	tagToField := make(map[string]int)
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("csv")
		if tag != "" {
			tagToField[tag] = i
		}
	}

	var mappings []fieldMapping
	for csvIdx, colName := range header {
		colName = strings.TrimSpace(colName)
		if fieldIdx, ok := tagToField[colName]; ok {
			mappings = append(mappings, fieldMapping{csvIndex: csvIdx, fieldIndex: fieldIdx})
		}
	}
	return mappings
}

// decodeRecord fills a struct T from a CSV record using the field mapping.
func decodeRecord[T any](record []string, fieldMap []fieldMapping) T {
	var t T
	v := reflect.ValueOf(&t).Elem()
	for _, fm := range fieldMap {
		if fm.csvIndex < len(record) {
			v.Field(fm.fieldIndex).SetString(record[fm.csvIndex])
		}
	}
	return t
}
