package queue

import (
	"testing"
	"time"
)

func TestValidateCronSpec(t *testing.T) {
	scheduler := &TaskScheduler{}

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
			err := scheduler.validateCronSpec(tt.cronSpec)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCronSpec() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseCronSpec(t *testing.T) {
	scheduler := &TaskScheduler{}

	tests := []struct {
		name          string
		cronSpec      string
		wantSecond    *CronField
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
			wantSecond:    &CronField{Min: 0, Max: 59, Values: []int{0}},
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
			wantSecond:    &CronField{Min: 0, Max: 59, Values: []int{0}},
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
			expr, err := scheduler.parseCronSpec(tt.cronSpec)
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

func TestParseCronFieldWithNames(t *testing.T) {
	scheduler := &TaskScheduler{}

	tests := []struct {
		name       string
		field      string
		min        int
		max        int
		nameMap    map[string]int
		wantValues []int
		wantErr    bool
	}{
		{
			name:       "month names",
			field:      "JAN,MAR,DEC",
			min:        1,
			max:        12,
			nameMap:    months,
			wantValues: []int{1, 3, 12},
			wantErr:    false,
		},
		{
			name:       "day of week names",
			field:      "MON-FRI",
			min:        0,
			max:        6,
			nameMap:    days,
			wantValues: []int{1, 2, 3, 4, 5},
			wantErr:    false,
		},
		{
			name:       "question mark",
			field:      "?",
			min:        1,
			max:        31,
			nameMap:    nil,
			wantValues: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31},
			wantErr:    false,
		},
		{
			name:       "mixed names and numbers",
			field:      "SUN,1,SAT",
			min:        0,
			max:        6,
			nameMap:    days,
			wantValues: []int{0, 1, 6},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cronField, err := scheduler.parseCronFieldWithNames(tt.field, tt.min, tt.max, tt.nameMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCronFieldWithNames() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && !equalIntSlices(cronField.Values, tt.wantValues) {
				t.Errorf("parseCronFieldWithNames() values = %v, want %v", cronField.Values, tt.wantValues)
			}
		})
	}
}

func TestParseStepField(t *testing.T) {
	scheduler := &TaskScheduler{}

	tests := []struct {
		name       string
		field      string
		min        int
		max        int
		wantValues []int
		wantErr    bool
	}{
		{
			name:       "simple step",
			field:      "*/5",
			min:        0,
			max:        59,
			wantValues: []int{0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55},
			wantErr:    false,
		},
		{
			name:       "range step",
			field:      "10-20/3",
			min:        0,
			max:        59,
			wantValues: []int{10, 13, 16, 19},
			wantErr:    false,
		},
		{
			name:       "start value step",
			field:      "5/10",
			min:        0,
			max:        59,
			wantValues: []int{5, 15, 25, 35, 45, 55},
			wantErr:    false,
		},
		{
			name:    "invalid step format",
			field:   "*/5/2",
			min:     0,
			max:     59,
			wantErr: true,
		},
		{
			name:    "invalid step value",
			field:   "*/a",
			min:     0,
			max:     59,
			wantErr: true,
		},
		{
			name:    "zero step",
			field:   "*/0",
			min:     0,
			max:     59,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cronField := CronField{Min: tt.min, Max: tt.max}
			result, err := scheduler.parseStepField(tt.field, tt.min, tt.max, cronField)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseStepField() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && !equalIntSlices(result.Values, tt.wantValues) {
				t.Errorf("parseStepField() values = %v, want %v", result.Values, tt.wantValues)
			}
		})
	}
}

func TestParseRangeField(t *testing.T) {
	scheduler := &TaskScheduler{}

	tests := []struct {
		name       string
		field      string
		min        int
		max        int
		wantValues []int
		wantErr    bool
	}{
		{
			name:       "valid range",
			field:      "1-5",
			min:        1,
			max:        31,
			wantValues: []int{1, 2, 3, 4, 5},
			wantErr:    false,
		},
		{
			name:       "single value range",
			field:      "15-15",
			min:        1,
			max:        31,
			wantValues: []int{15},
			wantErr:    false,
		},
		{
			name:    "invalid range format",
			field:   "1-5-10",
			min:     1,
			max:     31,
			wantErr: true,
		},
		{
			name:    "invalid start value",
			field:   "0-5",
			min:     1,
			max:     31,
			wantErr: true,
		},
		{
			name:    "invalid end value",
			field:   "1-32",
			min:     1,
			max:     31,
			wantErr: true,
		},
		{
			name:    "end less than start",
			field:   "10-5",
			min:     1,
			max:     31,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cronField := CronField{Min: tt.min, Max: tt.max}
			result, err := scheduler.parseRangeField(tt.field, tt.min, tt.max, cronField)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseRangeField() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && !equalIntSlices(result.Values, tt.wantValues) {
				t.Errorf("parseRangeField() values = %v, want %v", result.Values, tt.wantValues)
			}
		})
	}
}

func TestParseListField(t *testing.T) {
	scheduler := &TaskScheduler{}

	tests := []struct {
		name       string
		field      string
		min        int
		max        int
		wantValues []int
		wantErr    bool
	}{
		{
			name:       "valid list",
			field:      "1,3,5",
			min:        1,
			max:        31,
			wantValues: []int{1, 3, 5},
			wantErr:    false,
		},
		{
			name:       "list with spaces",
			field:      "1, 3, 5",
			min:        1,
			max:        31,
			wantValues: []int{1, 3, 5},
			wantErr:    false,
		},
		{
			name:       "single value list",
			field:      "15",
			min:        1,
			max:        31,
			wantValues: []int{15},
			wantErr:    false,
		},
		{
			name:    "invalid value in list",
			field:   "1,a,5",
			min:     1,
			max:     31,
			wantErr: true,
		},
		{
			name:    "out of range value",
			field:   "1,32,5",
			min:     1,
			max:     31,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cronField := CronField{Min: tt.min, Max: tt.max}
			result, err := scheduler.parseListField(tt.field, tt.min, tt.max, cronField)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseListField() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && !equalIntSlices(result.Values, tt.wantValues) {
				t.Errorf("parseListField() values = %v, want %v", result.Values, tt.wantValues)
			}
		})
	}
}

func TestCalculateNextCronRun(t *testing.T) {
	scheduler := &TaskScheduler{}

	tests := []struct {
		name     string
		cronSpec string
		after    time.Time
		wantTime time.Time
		wantErr  bool
	}{
		{
			name:     "every minute",
			cronSpec: "* * * * *",
			after:    time.Date(2023, 1, 1, 10, 30, 0, 0, time.UTC),
			wantTime: time.Date(2023, 1, 1, 10, 31, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "every hour at minute 0",
			cronSpec: "0 * * * *",
			after:    time.Date(2023, 1, 1, 10, 30, 0, 0, time.UTC),
			wantTime: time.Date(2023, 1, 1, 11, 0, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "daily at midnight",
			cronSpec: "0 0 * * *",
			after:    time.Date(2023, 1, 1, 10, 30, 0, 0, time.UTC),
			wantTime: time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "weekly on monday",
			cronSpec: "0 0 * * 1",
			after:    time.Date(2023, 1, 1, 10, 30, 0, 0, time.UTC), // Sunday
			wantTime: time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC),   // Monday
			wantErr:  false,
		},
		{
			name:     "with seconds",
			cronSpec: "30 * * * * *",
			after:    time.Date(2023, 1, 1, 10, 30, 0, 0, time.UTC),
			wantTime: time.Date(2023, 1, 1, 10, 30, 30, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "predefined @hourly",
			cronSpec: "@hourly",
			after:    time.Date(2023, 1, 1, 10, 30, 0, 0, time.UTC),
			wantTime: time.Date(2023, 1, 1, 11, 0, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "invalid cron spec",
			cronSpec: "invalid",
			after:    time.Date(2023, 1, 1, 10, 30, 0, 0, time.UTC),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTime, err := scheduler.calculateNextCronRun(tt.cronSpec, tt.after)
			if (err != nil) != tt.wantErr {
				t.Errorf("calculateNextCronRun() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && !gotTime.Equal(tt.wantTime) {
				t.Errorf("calculateNextCronRun() = %v, want %v", gotTime, tt.wantTime)
			}
		})
	}
}

func TestCronMatches(t *testing.T) {
	scheduler := &TaskScheduler{}

	tests := []struct {
		name     string
		cronSpec string
		testTime time.Time
		want     bool
	}{
		{
			name:     "match every minute",
			cronSpec: "* * * * *",
			testTime: time.Date(2023, 1, 1, 10, 30, 0, 0, time.UTC),
			want:     true,
		},
		{
			name:     "match specific minute",
			cronSpec: "30 * * * *",
			testTime: time.Date(2023, 1, 1, 10, 30, 0, 0, time.UTC),
			want:     true,
		},
		{
			name:     "no match specific minute",
			cronSpec: "30 * * * *",
			testTime: time.Date(2023, 1, 1, 10, 31, 0, 0, time.UTC),
			want:     false,
		},
		{
			name:     "match specific hour",
			cronSpec: "0 10 * * *",
			testTime: time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC),
			want:     true,
		},
		{
			name:     "no match specific hour",
			cronSpec: "0 10 * * *",
			testTime: time.Date(2023, 1, 1, 11, 0, 0, 0, time.UTC),
			want:     false,
		},
		{
			name:     "match specific day",
			cronSpec: "0 0 1 * *",
			testTime: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			want:     true,
		},
		{
			name:     "match specific month",
			cronSpec: "0 0 * 1 *",
			testTime: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			want:     true,
		},
		{
			name:     "match specific day of week",
			cronSpec: "0 0 * * 0",
			testTime: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), // Sunday
			want:     true,
		},
		{
			name:     "match with seconds",
			cronSpec: "30 0 0 * * *",
			testTime: time.Date(2023, 1, 1, 0, 0, 30, 0, time.UTC),
			want:     true,
		},
		{
			name:     "no match with seconds",
			cronSpec: "30 0 0 * * *",
			testTime: time.Date(2023, 1, 1, 0, 0, 31, 0, time.UTC),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := scheduler.parseCronSpec(tt.cronSpec)
			if err != nil {
				t.Fatalf("parseCronSpec() error = %v", err)
			}

			got := scheduler.cronMatches(expr, tt.testTime)
			if got != tt.want {
				t.Errorf("cronMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFieldMatches(t *testing.T) {
	scheduler := &TaskScheduler{}

	tests := []struct {
		name  string
		field CronField
		value int
		want  bool
	}{
		{
			name:  "match in values",
			field: CronField{Values: []int{1, 3, 5}},
			value: 3,
			want:  true,
		},
		{
			name:  "no match in values",
			field: CronField{Values: []int{1, 3, 5}},
			value: 4,
			want:  false,
		},
		{
			name:  "empty values",
			field: CronField{Values: []int{}},
			value: 1,
			want:  false,
		},
		{
			name:  "single value match",
			field: CronField{Values: []int{42}},
			value: 42,
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scheduler.fieldMatches(tt.field, tt.value)
			if got != tt.want {
				t.Errorf("fieldMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeFieldNames(t *testing.T) {
	scheduler := &TaskScheduler{}

	tests := []struct {
		name    string
		field   string
		nameMap map[string]int
		want    string
	}{
		{
			name:    "month names",
			field:   "JAN-DEC",
			nameMap: months,
			want:    "1-12",
		},
		{
			name:    "day names",
			field:   "MON,WED,FRI",
			nameMap: days,
			want:    "1,3,5",
		},
		{
			name:    "no names",
			field:   "1-5",
			nameMap: months,
			want:    "1-5",
		},
		{
			name:    "nil name map",
			field:   "JAN-DEC",
			nameMap: nil,
			want:    "JAN-DEC",
		},
		{
			name:    "mixed names and numbers",
			field:   "JAN,2,MAR",
			nameMap: months,
			want:    "1,2,3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scheduler.normalizeFieldNames(tt.field, tt.nameMap)
			if got != tt.want {
				t.Errorf("normalizeFieldNames() = %v, want %v", got, tt.want)
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
	scheduler := &TaskScheduler{}
	cronSpec := "*/5 9-17 * * 1-5"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := scheduler.parseCronSpec(cronSpec)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCalculateNextCronRun(b *testing.B) {
	scheduler := &TaskScheduler{}
	cronSpec := "*/5 9-17 * * 1-5"
	after := time.Date(2023, 1, 1, 10, 30, 0, 0, time.UTC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := scheduler.calculateNextCronRun(cronSpec, after)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCronMatches(b *testing.B) {
	scheduler := &TaskScheduler{}
	expr, err := scheduler.parseCronSpec("*/5 9-17 * * 1-5")
	if err != nil {
		b.Fatal(err)
	}
	testTime := time.Date(2023, 1, 1, 10, 30, 0, 0, time.UTC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = scheduler.cronMatches(expr, testTime)
	}
}
