package queue_test

import (
	"testing"

	"github.com/Valentin-Kaiser/go-core/queue"
)

func TestValidateCronSpec(t *testing.T) {
	t.Parallel()
	scheduler := &queue.TaskScheduler{}

	tests := []struct {
		name     string
		cronSpec string
		wantErr  bool
	}{
		// Valid 5-field expressions
		{"valid 5-field basic", "0 0 * * *", false},
		{"valid 5-field with ranges", "0 9-17 * * 1-5", false},
		{"valid 5-field with steps", "*/15 * * * *", false},
		{"valid 5-field with lists", "0 9,12,15 * * *", false},
		{"valid 5-field mixed", "0 9-17/2 * * 1-5", false},

		// Valid 6-field expressions
		{"valid 6-field basic", "0 0 0 * * *", false},
		{"valid 6-field with ranges", "0 0 9-17 * * 1-5", false},
		{"valid 6-field with steps", "*/30 */15 * * * *", false},
		{"valid 6-field with lists", "0 0 9,12,15 * * *", false},

		// Predefined expressions
		{"predefined @yearly", "@yearly", false},
		{"predefined @annually", "@annually", false},
		{"predefined @monthly", "@monthly", false},
		{"predefined @weekly", "@weekly", false},
		{"predefined @daily", "@daily", false},
		{"predefined @midnight", "@midnight", false},
		{"predefined @hourly", "@hourly", false},

		// Invalid expressions
		{"invalid field count", "0 0 0 0", true},
		{"invalid field count too many", "0 0 0 0 0 0 0", true},
		{"invalid minute range", "60 0 * * *", true},
		{"invalid hour range", "0 24 * * *", true},
		{"invalid day range", "0 0 0 * *", true},
		{"invalid month range", "0 0 * 13 *", true},
		{"invalid day of week range", "0 0 * * 7", true},
		{"invalid step format", "0 0 */a * *", true},
		{"invalid range format", "0 0 1-a * *", true},
		{"invalid list format", "0 0 1,a * *", true},
		{"empty expression", "", true},
		{"invalid predefined", "@invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := scheduler.ValidateCronSpec(tt.cronSpec)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCronSpec() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseCronSpec(t *testing.T) {
	t.Parallel()
	scheduler := &queue.TaskScheduler{}

	tests := []struct {
		name          string
		cronSpec      string
		wantSecond    *queue.CronField
		wantMinute    []int
		wantHour      []int
		wantDay       []int
		wantMonth     []int
		wantDayOfWeek []int
		wantErr       bool
	}{
		{
			name:          "basic 5-field",
			cronSpec:      "0 0 * * *",
			wantSecond:    nil,
			wantMinute:    []int{0},
			wantHour:      []int{0},
			wantDay:       []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31},
			wantMonth:     []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			wantDayOfWeek: []int{0, 1, 2, 3, 4, 5, 6},
			wantErr:       false,
		},
		{
			name:          "basic 6-field",
			cronSpec:      "0 0 0 * * *",
			wantSecond:    &queue.CronField{Min: 0, Max: 59, Values: []int{0}},
			wantMinute:    []int{0},
			wantHour:      []int{0},
			wantDay:       []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31},
			wantMonth:     []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			wantDayOfWeek: []int{0, 1, 2, 3, 4, 5, 6},
			wantErr:       false,
		},
		{
			name:          "range expression",
			cronSpec:      "0 9-17 * * 1-5",
			wantSecond:    nil,
			wantMinute:    []int{0},
			wantHour:      []int{9, 10, 11, 12, 13, 14, 15, 16, 17},
			wantDay:       []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31},
			wantMonth:     []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			wantDayOfWeek: []int{1, 2, 3, 4, 5},
			wantErr:       false,
		},
		{
			name:          "step expression",
			cronSpec:      "*/15 * * * *",
			wantSecond:    nil,
			wantMinute:    []int{0, 15, 30, 45},
			wantHour:      []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23},
			wantDay:       []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31},
			wantMonth:     []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			wantDayOfWeek: []int{0, 1, 2, 3, 4, 5, 6},
			wantErr:       false,
		},
		{
			name:          "list expression",
			cronSpec:      "0 9,12,15 * * *",
			wantSecond:    nil,
			wantMinute:    []int{0},
			wantHour:      []int{9, 12, 15},
			wantDay:       []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31},
			wantMonth:     []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			wantDayOfWeek: []int{0, 1, 2, 3, 4, 5, 6},
			wantErr:       false,
		},
		{
			name:          "predefined @hourly",
			cronSpec:      "@hourly",
			wantSecond:    &queue.CronField{Min: 0, Max: 59, Values: []int{0}},
			wantMinute:    []int{0},
			wantHour:      []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23},
			wantDay:       []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31},
			wantMonth:     []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			wantDayOfWeek: []int{0, 1, 2, 3, 4, 5, 6},
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			expr, err := scheduler.ParseCronSpec(tt.cronSpec)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCronSpec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Check seconds field
			if tt.wantSecond == nil {
				if expr.Second != nil {
					t.Errorf("parseCronSpec() expected nil second field, got %v", expr.Second)
				}
			} else {
				if expr.Second == nil {
					t.Errorf("parseCronSpec() expected second field, got nil")
				} else if !equalIntSlices(expr.Second.Values, tt.wantSecond.Values) {
					t.Errorf("parseCronSpec() second values = %v, want %v", expr.Second.Values, tt.wantSecond.Values)
				}
			}

			// Check other fields
			if !equalIntSlices(expr.Minute.Values, tt.wantMinute) {
				t.Errorf("parseCronSpec() minute values = %v, want %v", expr.Minute.Values, tt.wantMinute)
			}
			if !equalIntSlices(expr.Hour.Values, tt.wantHour) {
				t.Errorf("parseCronSpec() hour values = %v, want %v", expr.Hour.Values, tt.wantHour)
			}
			if !equalIntSlices(expr.Day.Values, tt.wantDay) {
				t.Errorf("parseCronSpec() day values = %v, want %v", expr.Day.Values, tt.wantDay)
			}
			if !equalIntSlices(expr.Month.Values, tt.wantMonth) {
				t.Errorf("parseCronSpec() month values = %v, want %v", expr.Month.Values, tt.wantMonth)
			}
			if !equalIntSlices(expr.DayOfWeek.Values, tt.wantDayOfWeek) {
				t.Errorf("parseCronSpec() dayOfWeek values = %v, want %v", expr.DayOfWeek.Values, tt.wantDayOfWeek)
			}
		})
	}
}

// Helper function to compare two int slices
func equalIntSlices(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

// Benchmarks
func BenchmarkParseCronSpec(b *testing.B) {
	scheduler := &queue.TaskScheduler{}
	cronSpec := "*/5 9-17 * * 1-5"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := scheduler.ParseCronSpec(cronSpec)
		if err != nil {
			b.Fatal(err)
		}
	}
}
