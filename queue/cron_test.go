package queue

import (
	"testing"
	"time"
)

func TestCronSpec_Validation(t *testing.T) {
	scheduler := NewTaskScheduler()

	testCases := []struct {
		name  string
		spec  string
		valid bool
	}{
		{"valid every minute", "* * * * *", true},
		{"valid every 5 minutes", "*/5 * * * *", true},
		{"valid daily at midnight", "0 0 * * *", true},
		{"valid hourly", "0 * * * *", true},
		{"valid range", "0-5 * * * *", true},
		{"valid list", "0,15,30,45 * * * *", true},
		{"valid complex", "0,15,30,45 9-17 * * 1-5", true},
		{"invalid too few fields", "* * *", false},
		{"invalid too many fields", "* * * * * *", false},
		{"invalid minute range", "60 * * * *", false},
		{"invalid hour range", "0 25 * * *", false},
		{"invalid day range", "0 0 32 * *", false},
		{"invalid month range", "0 0 1 13 *", false},
		{"invalid day-of-week range", "0 0 * * 7", false},
		{"invalid step", "*/0 * * * *", false},
		{"invalid range format", "0-5-10 * * * *", false},
		{"invalid number", "abc * * * *", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := scheduler.validateCronSpec(tc.spec)
			if tc.valid && err != nil {
				t.Errorf("expected valid spec '%s' to pass, got error: %v", tc.spec, err)
			}
			if !tc.valid && err == nil {
				t.Errorf("expected invalid spec '%s' to fail, but it passed", tc.spec)
			}
		})
	}
}

func TestCronField_Parsing(t *testing.T) {
	scheduler := NewTaskScheduler()

	testCases := []struct {
		name     string
		field    string
		min      int
		max      int
		expected []int
	}{
		{"wildcard", "*", 0, 59, generateRange(0, 59)},
		{"single value", "5", 0, 59, []int{5}},
		{"range", "5-10", 0, 59, []int{5, 6, 7, 8, 9, 10}},
		{"list", "1,3,5", 0, 59, []int{1, 3, 5}},
		{"step all", "*/5", 0, 59, []int{0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55}},
		{"step range", "10-30/5", 0, 59, []int{10, 15, 20, 25, 30}},
		{"step from value", "5/10", 0, 59, []int{5, 15, 25, 35, 45, 55}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			field, err := scheduler.parseCronField(tc.field, tc.min, tc.max)
			if err != nil {
				t.Fatalf("failed to parse field '%s': %v", tc.field, err)
			}

			if len(field.Values) != len(tc.expected) {
				t.Errorf("expected %d values, got %d", len(tc.expected), len(field.Values))
				return
			}

			for i, expected := range tc.expected {
				if field.Values[i] != expected {
					t.Errorf("expected value %d at index %d, got %d", expected, i, field.Values[i])
				}
			}
		})
	}
}

func TestCronExpression_NextRun(t *testing.T) {
	scheduler := NewTaskScheduler()

	testCases := []struct {
		name     string
		spec     string
		after    time.Time
		expected time.Time
	}{
		{
			name:     "every minute",
			spec:     "* * * * *",
			after:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			expected: time.Date(2024, 1, 1, 12, 1, 0, 0, time.UTC),
		},
		{
			name:     "every 5 minutes",
			spec:     "*/5 * * * *",
			after:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			expected: time.Date(2024, 1, 1, 12, 5, 0, 0, time.UTC),
		},
		{
			name:     "daily at midnight",
			spec:     "0 0 * * *",
			after:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			expected: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "hourly at minute 30",
			spec:     "30 * * * *",
			after:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			expected: time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC),
		},
		{
			name:     "weekdays at 9 AM",
			spec:     "0 9 * * 1-5",
			after:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), // Monday
			expected: time.Date(2024, 1, 2, 9, 0, 0, 0, time.UTC),  // Tuesday
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			next, err := scheduler.calculateNextCronRun(tc.spec, tc.after)
			if err != nil {
				t.Fatalf("failed to calculate next run: %v", err)
			}

			if !next.Equal(tc.expected) {
				t.Errorf("expected next run %v, got %v", tc.expected, next)
			}
		})
	}
}

func TestCronExpression_Matching(t *testing.T) {
	scheduler := NewTaskScheduler()

	testCases := []struct {
		name    string
		spec    string
		time    time.Time
		matches bool
	}{
		{
			name:    "every minute matches",
			spec:    "* * * * *",
			time:    time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC),
			matches: true,
		},
		{
			name:    "specific minute matches",
			spec:    "30 * * * *",
			time:    time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC),
			matches: true,
		},
		{
			name:    "specific minute doesn't match",
			spec:    "30 * * * *",
			time:    time.Date(2024, 1, 1, 12, 15, 0, 0, time.UTC),
			matches: false,
		},
		{
			name:    "daily at midnight matches",
			spec:    "0 0 * * *",
			time:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			matches: true,
		},
		{
			name:    "daily at midnight doesn't match",
			spec:    "0 0 * * *",
			time:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			matches: false,
		},
		{
			name:    "weekday matches",
			spec:    "0 9 * * 1-5",
			time:    time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC), // Monday
			matches: true,
		},
		{
			name:    "weekend doesn't match weekday",
			spec:    "0 9 * * 1-5",
			time:    time.Date(2024, 1, 6, 9, 0, 0, 0, time.UTC), // Saturday
			matches: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := scheduler.parseCronSpec(tc.spec)
			if err != nil {
				t.Fatalf("failed to parse cron spec: %v", err)
			}

			matches := scheduler.cronMatches(expr, tc.time)
			if matches != tc.matches {
				t.Errorf("expected match %v, got %v", tc.matches, matches)
			}
		})
	}
}

func TestCronExpression_ComplexSchedules(t *testing.T) {
	scheduler := NewTaskScheduler()

	testCases := []struct {
		name        string
		spec        string
		description string
	}{
		{"business_hours", "0 9-17 * * 1-5", "Every hour from 9 AM to 5 PM, Monday to Friday"},
		{"quarter_hours", "0,15,30,45 * * * *", "Every 15 minutes"},
		{"monthly_first", "0 0 1 * *", "Monthly on the 1st at midnight"},
		{"twice_daily", "0 8,20 * * *", "Twice daily at 8 AM and 8 PM"},
		{"workday_lunch", "0 12 * * 1-5", "Weekdays at noon"},
		{"weekend_morning", "0 10 * * 0,6", "Weekend mornings at 10 AM"},
		{"every_other_hour", "0 */2 * * *", "Every other hour"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := scheduler.validateCronSpec(tc.spec)
			if err != nil {
				t.Errorf("valid cron spec '%s' failed validation: %v", tc.spec, err)
			}

			// Test that we can calculate next run
			_, err = scheduler.calculateNextCronRun(tc.spec, time.Now())
			if err != nil {
				t.Errorf("failed to calculate next run for '%s': %v", tc.spec, err)
			}
		})
	}
}

// Helper function to generate a range of integers
func generateRange(min, max int) []int {
	result := make([]int, max-min+1)
	for i := min; i <= max; i++ {
		result[i-min] = i
	}
	return result
}
