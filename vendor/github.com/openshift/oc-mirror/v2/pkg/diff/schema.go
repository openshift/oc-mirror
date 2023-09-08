package diff

type SequenceSchema struct {
	Title    string   `toml:"title"`
	Owner    string   `toml:"owner"`
	Sequence Sequence `toml:"sequence"`
}

type Item struct {
	Value          int    `toml:"value"`
	Timestamp      int64  `toml:"timestamp"`
	Imagesetconfig string `toml:"imagesetconfig"`
	// this looks weird current used for previous
	// imagesetconfigs - its a simple way to track the most recent
	Current     bool   `toml:"current"`
	Destination string `toml:"destination"`
}

type Sequence struct {
	Item []Item `toml:"item"`
}
