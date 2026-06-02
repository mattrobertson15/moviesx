package store

// Role is one credit entry on a movie (cast or crew).
type Role struct {
	Order      int      `json:"order"`
	ActorID    string   `json:"actorId"`
	Name       string   `json:"name"`
	Category   string   `json:"category"`
	Characters []string `json:"characters,omitempty"`
	Job        string   `json:"job,omitempty"`
}

// ActorMovie is a movie entry in an actor's film list (denormalized).
type ActorMovie struct {
	MovieID string `json:"movieId"`
	Title   string `json:"title"`
}

// Movie is the in-memory enriched movie record (raw fields + pre-joined rating).
// Not serialized directly; handlers project into MovieListItem or MovieDetail.
type Movie struct {
	ID      string
	Title   string
	Year    int
	Runtime int
	Genres  []string
	Roles   []Role
	Rating  float64
	Votes   int
}

// Actor is an actor/crew record parsed from actors.json and used in API responses.
type Actor struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	BirthYear  int          `json:"birthYear"`
	DeathYear  int          `json:"deathYear"`
	Profession []string     `json:"profession"`
	Movies     []ActorMovie `json:"movies"`
}

// MovieListItem is the response shape for GET /api/movies list items (D2: cast-only roles).
type MovieListItem struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Year    int      `json:"year"`
	Runtime int      `json:"runtime"`
	Genres  []string `json:"genres"`
	Cast    []Role   `json:"cast"`
	Rating  float64  `json:"rating"`
	Votes   int      `json:"votes"`
}

// MovieDetail is the response shape for GET /api/movies/{id} (all roles).
type MovieDetail struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Year    int      `json:"year"`
	Runtime int      `json:"runtime"`
	Genres  []string `json:"genres"`
	Roles   []Role   `json:"roles"`
	Rating  float64  `json:"rating"`
	Votes   int      `json:"votes"`
}

// Page is the pagination envelope for list endpoints (D1 decision).
type Page[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}
