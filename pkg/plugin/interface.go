package plugin

type Plugin interface {
	BeforeCapture() error
	AfterCapture() error
}
