package notify

type Notifier interface {
	Notify(kind, name, problem string)
}
