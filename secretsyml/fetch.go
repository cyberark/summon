package secretsyml

// Interface that a secrets fetcher must implement
type Fetch func(string) (string, error)
