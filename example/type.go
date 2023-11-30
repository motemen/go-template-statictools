package example

type Data struct {
	Meta  Meta
	Items []Item
}

type Meta struct {
	Title string
}

type Item struct {
	Name  string
	Field map[string]string
}

func (i Item) Method(x string) string {
	return "method " + x
}
