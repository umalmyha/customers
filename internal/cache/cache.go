package cache

type CacheUpdater interface {
	Listen() error
	Stop()
}
