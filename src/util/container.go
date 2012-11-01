package util

type NameValuePair struct {
	Name  string
	Value string
}

type ListSelector struct {
	cursor int
	values []interface{}
}

func (se *ListSelector) ArrayValues() []interface{} {
	return se.values
}

func (se *ListSelector) Pop() interface{} {
    if len(se.values) > 0{
       v := se.values[0]
       se.values = se.values[1:]
       return v
    }
	return nil
}

func (se *ListSelector) Select() interface{} {
	if len(se.values) == 0 {
		return nil
	}
	if se.cursor >= len(se.values) {
		se.cursor = 0
	}
	val := se.values[se.cursor]
	se.cursor++
	return val
}

func (se *ListSelector) Add(v interface{}) {
	se.values = append(se.values, v)
}

func (se *ListSelector) Size() int {
	return len(se.values)
}
