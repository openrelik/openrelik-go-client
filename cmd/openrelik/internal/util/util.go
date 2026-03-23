package util

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
)

// GenerateUUID generates a random UUID v4 (hex, no hyphens).
func GenerateUUID() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")
}

// PrintStruct nicely prints the fields of a struct to stdout.
func PrintStruct(s interface{}) {
	FprintStruct(os.Stdout, s)
}

// FprintStruct nicely prints the fields of a struct or a slice of structs to the given writer.
func FprintStruct(w io.Writer, s interface{}) {
	v := reflect.ValueOf(s)

	// Handle pointer
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			fmt.Fprintf(w, "<nil>\n")
			return
		}
		v = v.Elem()
	}

	// Handle slice
	if v.Kind() == reflect.Slice {
		for i := 0; i < v.Len(); i++ {
			FprintStruct(w, v.Index(i).Interface())
			if i < v.Len()-1 {
				fmt.Fprintln(w, "---")
			}
		}
		return
	}

	if v.Kind() != reflect.Struct {
		fmt.Fprintf(w, "%v\n", s)
		return
	}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)

		// Handle unexported fields
		if !field.IsExported() {
			continue
		}

		var val interface{}
		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				val = "<nil>"
			} else {
				val = value.Elem().Interface()
			}
		} else {
			val = value.Interface()
		}

		fmt.Fprintf(w, "%-20s: %v\n", field.Name, val)
	}
}

// ProgressTracker handles the display of a progress bar.
type ProgressTracker struct {
	TotalBytes  int64
	TotalChunks int
	Action      string
	Writer      io.Writer

	currentBytes  int64
	currentChunks int
	retries       int
	startTime     time.Time
	samples       []progressSample

	lastChunkTime  time.Time
	chunkDurations []time.Duration

	lastETACalc time.Time
	cachedETA   string
}

type progressSample struct {
	timestamp time.Time
	bytes     int64
}

func NewProgressTracker(writer io.Writer, totalBytes int64, action string) *ProgressTracker {
	now := time.Now()
	return &ProgressTracker{
		Writer:        writer,
		TotalBytes:    totalBytes,
		Action:        action,
		startTime:     now,
		lastChunkTime: now,
	}
}

func (pt *ProgressTracker) Update(bytesSent int64) {
	pt.currentBytes = bytesSent
	pt.display()
}

func (pt *ProgressTracker) IncrementChunk() {
	now := time.Now()
	duration := now.Sub(pt.lastChunkTime)
	pt.chunkDurations = append(pt.chunkDurations, duration)
	// Keep last 20 chunks for moving average
	if len(pt.chunkDurations) > 20 {
		pt.chunkDurations = pt.chunkDurations[1:]
	}
	pt.lastChunkTime = now
	pt.currentChunks++
	pt.display()
}

func (pt *ProgressTracker) IncrementRetry() {
	pt.retries++
	pt.display()
}

func (pt *ProgressTracker) SetTotalChunks(total int) {
	pt.TotalChunks = total
}

func (pt *ProgressTracker) Finish() {
	if pt.Writer != nil {
		fmt.Fprint(pt.Writer, "\n")
	}
}

func (pt *ProgressTracker) display() {
	if pt.Writer == nil {
		return
	}

	now := time.Now()
	var speed float64

	// Update Speed (sliding window)
	if pt.Action == "Downloading" {
		if len(pt.samples) == 0 || now.Sub(pt.samples[len(pt.samples)-1].timestamp) > 100*time.Millisecond {
			pt.samples = append(pt.samples, progressSample{now, pt.currentBytes})
		}
		cutoff := now.Add(-5 * time.Second)
		for len(pt.samples) > 2 && pt.samples[1].timestamp.Before(cutoff) {
			pt.samples = pt.samples[1:]
		}
		if len(pt.samples) > 1 {
			first := pt.samples[0]
			last := pt.samples[len(pt.samples)-1]
			dt := last.timestamp.Sub(first.timestamp).Seconds()
			if dt > 0 {
				speed = float64(last.bytes-first.bytes) / dt
			}
		}
	} else {
		elapsed := now.Sub(pt.startTime)
		if elapsed > 0 {
			speed = float64(pt.currentBytes) / elapsed.Seconds()
		}
	}

	// Recalculate ETA at most once per second
	if now.Sub(pt.lastETACalc) >= 1*time.Second {
		eta := ""
		if pt.Action == "Downloading" {
			if pt.TotalBytes > 0 && pt.currentBytes < pt.TotalBytes {
				if speed > 0 {
					remaining := pt.TotalBytes - pt.currentBytes
					d := time.Duration(float64(remaining)/speed) * time.Second
					eta = "ETA " + formatDuration(d)
				} else {
					eta = "ETA --"
				}
			}
		} else {
			if pt.TotalChunks > 0 && pt.currentChunks < pt.TotalChunks {
				if len(pt.chunkDurations) > 0 {
					var totalDur time.Duration
					for _, d := range pt.chunkDurations {
						totalDur += d
					}
					avgDur := totalDur / time.Duration(len(pt.chunkDurations))
					remainingChunks := pt.TotalChunks - pt.currentChunks
					d := avgDur * time.Duration(remainingChunks)
					eta = "ETA " + formatDuration(d)
				} else {
					eta = "ETA --"
				}
			}
		}
		pt.cachedETA = eta
		pt.lastETACalc = now
	}

	// Always clear ETA if finished
	if pt.currentBytes >= pt.TotalBytes {
		pt.cachedETA = ""
	}

	const barWidth = 30
	percent := float64(0)
	if pt.TotalBytes > 0 {
		percent = float64(pt.currentBytes) / float64(pt.TotalBytes)
	}
	if percent > 1 {
		percent = 1
	}

	completed := int(percent * barWidth)
	if completed < 0 {
		completed = 0
	}
	remaining := barWidth - completed

	bar := fmt.Sprintf("[%s%s]",
		strings.Repeat("=", completed),
		strings.Repeat("-", remaining),
	)

	// Build statistics
	stats := []string{fmt.Sprintf("%s/s", formatBytes(int64(speed)))}
	stats = append(stats, fmt.Sprintf("%s/%s", formatBytes(pt.currentBytes), formatBytes(pt.TotalBytes)))

	if pt.TotalChunks > 0 {
		stats = append(stats, fmt.Sprintf("%d/%d chunks", pt.currentChunks, pt.TotalChunks))
	}

	if pt.cachedETA != "" {
		stats = append(stats, pt.cachedETA)
	}

	if pt.retries > 0 {
		stats = append(stats, fmt.Sprintf("%d retries", pt.retries))
	}

	fmt.Fprintf(pt.Writer, "\r%s %s %3.0f%% (%s)\033[K", pt.Action, bar, percent*100, strings.Join(stats, " · "))
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// ProgressReader wraps an io.Reader and reports progress via a tracker.
type ProgressReader struct {
	io.Reader
	Tracker *ProgressTracker
}

func NewProgressReader(r io.Reader, total int64, w io.Writer) *ProgressReader {
	return &ProgressReader{
		Reader:  r,
		Tracker: NewProgressTracker(w, total, "Downloading"),
	}
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.Tracker.currentBytes += int64(n)
	pr.Tracker.display()

	if err == io.EOF {
		pr.Tracker.Finish()
	}

	return n, err
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// Confirm asks the user for confirmation (y/n) via the given reader and writer.
func Confirm(w io.Writer, r io.Reader, message string) (bool, error) {
	fmt.Fprintf(w, "%s [y/N]: ", message)
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, err
		}
		return false, nil // EOF
	}
	response := strings.ToLower(strings.TrimSpace(scanner.Text()))
	if response == "y" || response == "yes" {
		return true, nil
	}
	return false, nil
}

// FprintJSON prints the given interface as a pretty-printed JSON string.
func FprintJSON(w io.Writer, s interface{}) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(s)
}
