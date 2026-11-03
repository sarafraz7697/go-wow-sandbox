package tbc

import (
	"context"
	"wowserver/internal/domain/account"
	"wowserver/internal/domain/realm"
)

type AccessStore interface {
	IPAllowed(context.Context, string) (bool, error)
}

type Store interface {
	Find(context.Context, string) (account.Account, error)
	Publish(context.Context, account.Account, [40]byte, uint16, string) error
	Failed(context.Context, uint64) error
}

type SessionStore interface {
	Session(context.Context, uint64, uint16, string) ([40]byte, error)
}

type RealmStore interface {
	List(context.Context, uint64, uint16) ([]realm.Realm, error)
}
type RoutedRealmStore interface {
	ListForClient(context.Context, uint64, uint16, string) ([]realm.Realm, error)
}
