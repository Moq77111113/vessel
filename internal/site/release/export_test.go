package release

func NewRecordsFlushedBy(dir string, flush func()) *Records {
	return &Records{dir: dir, flush: flush}
}
