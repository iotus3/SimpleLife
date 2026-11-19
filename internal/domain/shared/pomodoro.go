package shared

// How long one pomodoro runs, in minutes: one minute to three hours, defined once because a
// task and a template both store such a length. Zero means "no length of my own".
const (
	FocusMinutesMin = 1
	FocusMinutesMax = 180
)
