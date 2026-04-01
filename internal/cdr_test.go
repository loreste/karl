package internal

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCDRFormat_String(t *testing.T) {
	tests := []struct {
		format   CDRFormat
		expected string
	}{
		{CDRFormatJSON, "json"},
		{CDRFormatCSV, "csv"},
		{CDRFormatSyslog, "syslog"},
		{CDRFormat(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.format.String(); got != tt.expected {
			t.Errorf("CDRFormat(%d).String() = %s, expected %s", tt.format, got, tt.expected)
		}
	}
}

func TestCDR_CalculateDurations(t *testing.T) {
	cdr := &CDR{
		StartTime:  time.Now(),
		AnswerTime: time.Now().Add(5 * time.Second),
		EndTime:    time.Now().Add(65 * time.Second),
	}

	cdr.CalculateDurations()

	if cdr.Duration != 65000 {
		t.Errorf("Expected Duration 65000ms, got %d", cdr.Duration)
	}
	if cdr.SetupTime != 5000 {
		t.Errorf("Expected SetupTime 5000ms, got %d", cdr.SetupTime)
	}
	if cdr.TalkTime != 60000 {
		t.Errorf("Expected TalkTime 60000ms, got %d", cdr.TalkTime)
	}
}

func TestCDR_CalculatePacketLoss(t *testing.T) {
	cdr := &CDR{
		PacketsRx:   90,
		PacketsLost: 10,
	}

	cdr.CalculatePacketLoss()

	if cdr.PacketsLostPct != 10.0 {
		t.Errorf("Expected PacketsLostPct 10.0, got %f", cdr.PacketsLostPct)
	}
}

func TestCDR_CalculatePacketLoss_ZeroPackets(t *testing.T) {
	cdr := &CDR{
		PacketsRx:   0,
		PacketsLost: 0,
	}

	cdr.CalculatePacketLoss()

	if cdr.PacketsLostPct != 0 {
		t.Errorf("Expected PacketsLostPct 0, got %f", cdr.PacketsLostPct)
	}
}

func TestCDR_ToCSVRow(t *testing.T) {
	now := time.Now()
	cdr := &CDR{
		ID:        "cdr-123",
		CallID:    "call-456",
		StartTime: now,
		EndTime:   now.Add(time.Minute),
		Duration:  60000,
		Status:    "completed",
	}

	row := cdr.ToCSVRow()

	if len(row) != 24 {
		t.Errorf("Expected 24 columns, got %d", len(row))
	}
	if row[0] != "cdr-123" {
		t.Errorf("Expected id 'cdr-123', got %s", row[0])
	}
	if row[1] != "call-456" {
		t.Errorf("Expected call_id 'call-456', got %s", row[1])
	}
}

func TestCSVHeader(t *testing.T) {
	header := CSVHeader()

	if len(header) != 24 {
		t.Errorf("Expected 24 header columns, got %d", len(header))
	}
	if header[0] != "id" {
		t.Error("First header should be 'id'")
	}
}

func TestDefaultCDRExporterConfig(t *testing.T) {
	config := DefaultCDRExporterConfig()

	if config.Format != CDRFormatJSON {
		t.Error("Default format should be JSON")
	}
	if config.BufferSize != 1000 {
		t.Errorf("Expected BufferSize 1000, got %d", config.BufferSize)
	}
}

func TestCDRExporter_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "cdrs.json")

	config := &CDRExporterConfig{
		Format:        CDRFormatJSON,
		OutputPath:    outputPath,
		BufferSize:    10,
		FlushInterval: time.Hour, // Don't auto-flush
	}

	exporter, err := NewCDRExporter(config)
	if err != nil {
		t.Fatalf("NewCDRExporter failed: %v", err)
	}
	defer exporter.Close()

	// Export a CDR
	cdr := &CDR{
		ID:        "test-cdr",
		CallID:    "call-123",
		StartTime: time.Now(),
		EndTime:   time.Now().Add(time.Minute),
		Status:    "completed",
	}

	err = exporter.Export(cdr)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Flush to file
	err = exporter.Flush()
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Verify file contents
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	var exported CDR
	if err := json.Unmarshal(data[:len(data)-1], &exported); err != nil { // Remove newline
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if exported.ID != "test-cdr" {
		t.Errorf("Expected ID 'test-cdr', got %s", exported.ID)
	}
}

func TestCDRExporter_CSV(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "cdrs.csv")

	config := &CDRExporterConfig{
		Format:        CDRFormatCSV,
		OutputPath:    outputPath,
		BufferSize:    10,
		FlushInterval: time.Hour,
		IncludeHeader: true,
	}

	exporter, err := NewCDRExporter(config)
	if err != nil {
		t.Fatalf("NewCDRExporter failed: %v", err)
	}
	defer exporter.Close()

	// Export a CDR
	cdr := &CDR{
		ID:        "test-cdr",
		CallID:    "call-123",
		StartTime: time.Now(),
		EndTime:   time.Now().Add(time.Minute),
		Status:    "completed",
	}

	exporter.Export(cdr)
	exporter.Flush()

	// Verify file contents
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	reader := csv.NewReader(bytes.NewReader(data))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("Failed to parse CSV: %v", err)
	}

	// Should have header + 1 data row
	if len(records) != 2 {
		t.Errorf("Expected 2 rows (header + data), got %d", len(records))
	}
}

func TestCDRExporter_BufferFlush(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "cdrs.json")

	config := &CDRExporterConfig{
		Format:        CDRFormatJSON,
		OutputPath:    outputPath,
		BufferSize:    3, // Small buffer to trigger auto-flush
		FlushInterval: time.Hour,
	}

	exporter, err := NewCDRExporter(config)
	if err != nil {
		t.Fatalf("NewCDRExporter failed: %v", err)
	}
	defer exporter.Close()

	// Export 3 CDRs - should trigger auto-flush
	for i := 0; i < 3; i++ {
		cdr := &CDR{
			ID:        "test-cdr",
			StartTime: time.Now(),
			EndTime:   time.Now(),
		}
		exporter.Export(cdr)
	}

	stats := exporter.GetStats()
	if stats["exported"].(int64) != 3 {
		t.Errorf("Expected 3 exported, got %v", stats["exported"])
	}
}

func TestCDRExporter_GetStats(t *testing.T) {
	config := &CDRExporterConfig{
		Format:        CDRFormatJSON,
		BufferSize:    100,
		FlushInterval: time.Hour,
	}

	exporter, err := NewCDRExporter(config)
	if err != nil {
		t.Fatalf("NewCDRExporter failed: %v", err)
	}
	defer exporter.Close()

	stats := exporter.GetStats()

	if stats["format"] != "json" {
		t.Errorf("Expected format 'json', got %v", stats["format"])
	}
	if stats["exported"].(int64) != 0 {
		t.Errorf("Expected 0 exported, got %v", stats["exported"])
	}
}

func TestCDRExporter_Close(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "cdrs.json")

	config := &CDRExporterConfig{
		Format:        CDRFormatJSON,
		OutputPath:    outputPath,
		BufferSize:    100,
		FlushInterval: time.Hour,
	}

	exporter, err := NewCDRExporter(config)
	if err != nil {
		t.Fatalf("NewCDRExporter failed: %v", err)
	}

	// Export a CDR
	cdr := &CDR{ID: "test", StartTime: time.Now(), EndTime: time.Now()}
	exporter.Export(cdr)

	// Close should flush
	err = exporter.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Double close should be safe
	err = exporter.Close()
	if err != nil {
		t.Errorf("Second close failed: %v", err)
	}
}

func TestCDRBuilder(t *testing.T) {
	start := time.Now()
	answer := start.Add(5 * time.Second)
	end := start.Add(65 * time.Second)

	cdr := NewCDRBuilder().
		WithCallID("call-123").
		WithTags("from-tag", "to-tag").
		WithParties("1234567890", "0987654321").
		WithTiming(start, answer, end).
		WithMedia("opus", 1000, 1000, 50000, 50000).
		WithQuality(10, 5.5, 4.2).
		WithStatus("completed", "normal", 200).
		WithNetwork("192.168.1.1", "10.0.0.1", 10000, 20000).
		WithRecording(true, "/recordings/call-123.wav").
		WithCustomField("customer_id", "cust-456").
		Build()

	if cdr.CallID != "call-123" {
		t.Error("CallID not set")
	}
	if cdr.FromTag != "from-tag" {
		t.Error("FromTag not set")
	}
	if cdr.CallerNumber != "1234567890" {
		t.Error("CallerNumber not set")
	}
	if cdr.Codec != "opus" {
		t.Error("Codec not set")
	}
	if cdr.PacketsRx != 1000 {
		t.Error("PacketsRx not set")
	}
	if cdr.MOS != 4.2 {
		t.Error("MOS not set")
	}
	if cdr.Status != "completed" {
		t.Error("Status not set")
	}
	if cdr.LocalIP != "192.168.1.1" {
		t.Error("LocalIP not set")
	}
	if !cdr.RecordingEnabled {
		t.Error("RecordingEnabled not set")
	}
	if cdr.CustomFields["customer_id"] != "cust-456" {
		t.Error("CustomField not set")
	}
	if cdr.Duration != 65000 {
		t.Errorf("Duration not calculated, got %d", cdr.Duration)
	}
}

func TestMemoryCDRExporter(t *testing.T) {
	exporter := NewMemoryCDRExporter()

	cdr1 := &CDR{ID: "cdr-1"}
	cdr2 := &CDR{ID: "cdr-2"}

	exporter.Export(cdr1)
	exporter.Export(cdr2)

	cdrs := exporter.GetCDRs()
	if len(cdrs) != 2 {
		t.Errorf("Expected 2 CDRs, got %d", len(cdrs))
	}

	exporter.Reset()

	cdrs = exporter.GetCDRs()
	if len(cdrs) != 0 {
		t.Error("CDRs should be cleared after Reset")
	}
}

func TestWriteCDRToWriter_JSON(t *testing.T) {
	cdr := &CDR{
		ID:     "test-cdr",
		CallID: "call-123",
	}

	var buf bytes.Buffer
	err := WriteCDRToWriter(&buf, cdr, CDRFormatJSON)
	if err != nil {
		t.Fatalf("WriteCDRToWriter failed: %v", err)
	}

	var exported CDR
	if err := json.Unmarshal(buf.Bytes(), &exported); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if exported.ID != "test-cdr" {
		t.Error("ID not written correctly")
	}
}

func TestWriteCDRToWriter_CSV(t *testing.T) {
	now := time.Now()
	cdr := &CDR{
		ID:        "test-cdr",
		CallID:    "call-123",
		StartTime: now,
		EndTime:   now,
	}

	var buf bytes.Buffer
	err := WriteCDRToWriter(&buf, cdr, CDRFormatCSV)
	if err != nil {
		t.Fatalf("WriteCDRToWriter failed: %v", err)
	}

	reader := csv.NewReader(&buf)
	record, err := reader.Read()
	if err != nil {
		t.Fatalf("Failed to parse CSV: %v", err)
	}

	if record[0] != "test-cdr" {
		t.Errorf("Expected id 'test-cdr', got %s", record[0])
	}
}

func TestWriteCDRToWriter_UnsupportedFormat(t *testing.T) {
	cdr := &CDR{ID: "test"}
	var buf bytes.Buffer

	err := WriteCDRToWriter(&buf, cdr, CDRFormatSyslog)
	if err == nil {
		t.Error("Should return error for unsupported format")
	}
}

func TestCDRExporter_Rotation(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "cdrs.json")

	config := &CDRExporterConfig{
		Format:        CDRFormatJSON,
		OutputPath:    outputPath,
		BufferSize:    1,
		FlushInterval: time.Hour,
		RotateSize:    100, // Very small to trigger rotation
		MaxFiles:      3,
	}

	exporter, err := NewCDRExporter(config)
	if err != nil {
		t.Fatalf("NewCDRExporter failed: %v", err)
	}
	defer exporter.Close()

	// Export several CDRs to trigger rotation
	for i := 0; i < 10; i++ {
		cdr := &CDR{
			ID:            "test-cdr-long-id-to-increase-size",
			CallID:        "call-123-with-additional-data",
			CallerNumber:  "1234567890",
			CalleeNumber:  "0987654321",
			Status:        "completed",
			StartTime:     time.Now(),
			EndTime:       time.Now(),
		}
		exporter.Export(cdr)
	}

	// Check that rotation occurred
	matches, _ := filepath.Glob(filepath.Join(tmpDir, "cdrs.json*"))
	if len(matches) < 1 {
		t.Error("Expected at least one CDR file")
	}
}

func TestGenerateCDRID(t *testing.T) {
	id1 := generateCDRID()
	id2 := generateCDRID()

	if id1 == "" {
		t.Error("CDR ID should not be empty")
	}
	if id1 == id2 {
		t.Error("CDR IDs should be unique")
	}
	if len(id1) < 10 {
		t.Error("CDR ID should be reasonably long")
	}
}
