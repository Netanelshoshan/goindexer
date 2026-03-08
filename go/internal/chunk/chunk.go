package chunk

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b) - 1
	for n > 0 {
		b[i] = byte('0' + n%10)
		n /= 10
		i--
	}
	return string(b[i+1:])
}

type CodeChunk struct {
	Text       string
	FilePath   string
	SymbolName string
	SymbolType string
	StartLine  int
	EndLine    int
	Language   string
	Breadcrumb string // e.g. "File: user.go | Class: UserService | Method: Update"
}

func (c *CodeChunk) ID() string {
	return c.FilePath + ":" + itoa(c.StartLine) + ":" + itoa(c.EndLine)
}
