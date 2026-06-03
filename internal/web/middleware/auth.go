package middleware

import (
	"context"
	"net/http"

	"github.com/codehive/codehive/internal/models"
	"github.com/codehive/codehive/internal/store"
)

type contextKey string

const UserContextKey contextKey = "user"

type AuthMiddleware struct {
	sessionStore *store.SessionStore
	userStore    *store.UserStore
}

func NewAuthMiddleware(ss *store.SessionStore, us *store.UserStore) *AuthMiddleware {
	return &AuthMiddleware{sessionStore: ss, userStore: us}
}

func (m *AuthMiddleware) Required(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUser(r)
		if user == nil {
			cookie, err := r.Cookie("codehive_session")
			if err != nil || cookie.Value == "" {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			sess, err := m.sessionStore.Get(r.Context(), cookie.Value)
			if err != nil {
				http.SetCookie(w, &http.Cookie{Name: "codehive_session", MaxAge: -1, Path: "/"})
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			user, err = m.userStore.GetByID(r.Context(), sess.UserID)
			if err != nil {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			ctx := context.WithValue(r.Context(), UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *AuthMiddleware) Optional(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("codehive_session")
		if err == nil && cookie.Value != "" {
			sess, err := m.sessionStore.Get(r.Context(), cookie.Value)
			if err == nil {
				user, err := m.userStore.GetByID(r.Context(), sess.UserID)
				if err == nil {
					ctx := context.WithValue(r.Context(), UserContextKey, user)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func GetUser(r *http.Request) *models.User {
	if user, ok := r.Context().Value(UserContextKey).(*models.User); ok {
		return user
	}
	return nil
}
