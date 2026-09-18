package invoiced

// HourLine represents a single time entry for hour tracking.
type HourLine struct {
	Day         string  `json:"Day"`
	Start       string  `json:"Start"`
	Stop        string  `json:"Stop"`
	Hours       float64 `json:"Hours"`
	Description string  `json:"Description"`
}

// Hour represents a collection of hour entries for a project.
type Hour struct {
	Project  string     `json:"Project"`
	Name     string     `json:"Name"`
	Status   string     `json:"Status"`
	Total    string     `json:"Total"`
	Lines    []HourLine `json:"Lines"`
	FilePath string     `json:"-"` // Local file path, not sent to API
}
