package handlers

import (
	"html/template"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/codehive/codehive/internal/config"
	"github.com/codehive/codehive/internal/models"
	"github.com/codehive/codehive/internal/store"
	"github.com/codehive/codehive/internal/web/middleware"
	"golang.org/x/crypto/bcrypt"
)

var usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,40}$`)

type AuthHandler struct {
	userStore    *store.UserStore
	sessionStore *store.SessionStore
	cfg          *config.Config
	templates    *template.Template
}

func NewAuthHandler(us *store.UserStore, ss *store.SessionStore, cfg *config.Config, tmpl *template.Template) *AuthHandler {
	return &AuthHandler{userStore: us, sessionStore: ss, cfg: cfg, templates: tmpl}
}

func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.templates.ExecuteTemplate(w, "login.html", map[string]interface{}{
		"Error": r.URL.Query().Get("error"),
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	user, err := h.userStore.GetByUsername(r.Context(), username)
	if err != nil {
		user, err = h.userStore.GetByEmail(r.Context(), username)
	}
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		http.Redirect(w, r, "/login?error=Invalid+credentials", http.StatusSeeOther)
		return
	}

	sess := &models.Session{
		UserID:    user.ID,
		IPAddress: r.RemoteAddr,
		UserAgent: r.UserAgent(),
		ExpiresAt: time.Now().Add(time.Duration(h.cfg.Session.MaxAge) * time.Second),
	}
	if err := h.sessionStore.Create(r.Context(), sess); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "codehive_session",
		Value:    sess.ID,
		Path:     "/",
		MaxAge:   h.cfg.Session.MaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *AuthHandler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	h.templates.ExecuteTemplate(w, "register.html", map[string]interface{}{
		"Error": r.URL.Query().Get("error"),
	})
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")

	if !usernameRegex.MatchString(username) {
		http.Redirect(w, r, "/register?error=Invalid+username+(3-40+chars,+alphanumeric+_-)", http.StatusSeeOther)
		return
	}
	if len(password) < 8 {
		http.Redirect(w, r, "/register?error=Password+must+be+at+least+8+characters", http.StatusSeeOther)
		return
	}
	if password != confirm {
		http.Redirect(w, r, "/register?error=Passwords+do+not+match", http.StatusSeeOther)
		return
	}
	if !strings.Contains(email, "@") {
		http.Redirect(w, r, "/register?error=Invalid+email+address", http.StatusSeeOther)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	user := &models.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
	}

	if err := h.userStore.Create(r.Context(), user); err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			http.Redirect(w, r, "/register?error=Username+or+email+already+taken", http.StatusSeeOther)
			return
		}
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	sess := &models.Session{
		UserID:    user.ID,
		IPAddress: r.RemoteAddr,
		UserAgent: r.UserAgent(),
		ExpiresAt: time.Now().Add(time.Duration(h.cfg.Session.MaxAge) * time.Second),
	}
	h.sessionStore.Create(r.Context(), sess)
	http.SetCookie(w, &http.Cookie{
		Name:     "codehive_session",
		Value:    sess.ID,
		Path:     "/",
		MaxAge:   h.cfg.Session.MaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("codehive_session")
	if err == nil {
		h.sessionStore.Delete(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "codehive_session", MaxAge: -1, Path: "/"})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func CurrentUser(r *http.Request) *models.User {
	return middleware.GetUser(r)
}
