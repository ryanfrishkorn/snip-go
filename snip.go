package snip

type Snip struct {
	UUID string
}

func New() (Snip, error) {
	return Snip{
		UUID: CreateUUID(),
	}, nil
}

func CreateUUID() string {
	return "xxxx-yyyy-zzzz"
}
