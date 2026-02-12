package database

type rowScanner interface {
	Scan(dest ...any) error
}
