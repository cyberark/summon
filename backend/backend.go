package backend

type Backend interface {
	Fetch(string) (string, error)
}
