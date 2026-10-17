package domain

import (
	"context"
	"time"
)

type User struct {
	TelegramID int64
	Username   string
	Timezone   string
	CreatedAt  time.Time
}

type UserRepository interface {
	Upsert(ctx context.Context, user *User) error
	GetByID(ctx context.Context, telegramID int64) (*User, error)
}
