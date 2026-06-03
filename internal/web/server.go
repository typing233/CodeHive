package web

import (
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/codehive/codehive/internal/config"
	"github.com/codehive/codehive/internal/gitbackend"
	"github.com/codehive/codehive/internal/store"
	"github.com/codehive/codehive/internal/web/handlers"
	"github.com/codehive/codehive/internal/web/middleware"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	cfg          *config.Config
	userStore    *store.UserStore
	repoStore    *store.RepoStore
	issueStore   *store.IssueStore
	sessionStore *store.SessionStore
	tokenStore   *store.TokenStore
	gitSvc       *gitbackend.Service
	templates    *template.Template
}

func NewServer(cfg *config.Config, us *store.UserStore, rs *store.RepoStore, is *store.IssueStore, ss *store.SessionStore, ts *store.TokenStore, gs *gitbackend.Service) *Server {
	s := &Server{
		cfg:          cfg,
		userStore:    us,
		repoStore:    rs,
		issueStore:   is,
		sessionStore: ss,
		tokenStore:   ts,
		gitSvc:       gs,
	}
	s.loadTemplates()
	return s
}

func (s *Server) loadTemplates() {
	funcMap := template.FuncMap{
		"split":       strings.Split,
		"join":        strings.Join,
		"hasPrefix":   strings.HasPrefix,
		"hasSuffix":   strings.HasSuffix,
		"trimPrefix":  strings.TrimPrefix,
		"toLower":     strings.ToLower,
		"toUpper":     strings.ToUpper,
		"safeHTML":    func(s string) template.HTML { return template.HTML(s) },
		"pathJoin":    filepath.Join,
		"add":         func(a, b int) int { return a + b },
		"sub":         func(a, b int) int { return a - b },
		"mul":         func(a, b int) int { return a * b },
		"seq":         seq,
		"truncate":    truncate,
		"timeAgo":     timeAgo,
		"shortSHA":    func(s string) string { if len(s) > 7 { return s[:7] }; return s },
		"emojiToHTML": emojiToHTML,
		"deref": func(p interface{}) interface{} { return p },
	}

	tmpl := template.New("").Funcs(funcMap)

	var files []string
	filepath.WalkDir("internal/web/templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".html") {
			files = append(files, path)
		}
		return nil
	})

	if len(files) > 0 {
		_, err := tmpl.ParseFiles(files...)
		if err != nil {
			log.Fatalf("Failed to parse templates: %v", err)
		}
	}
	s.templates = tmpl
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Compress(5))

	authMW := middleware.NewAuthMiddleware(s.sessionStore, s.userStore)

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("internal/web/static"))))

	authHandler := handlers.NewAuthHandler(s.userStore, s.sessionStore, s.cfg, s.templates)
	repoHandler := handlers.NewRepoHandler(s.repoStore, s.userStore, s.gitSvc, s.cfg, s.templates)
	browseHandler := handlers.NewBrowseHandler(s.repoStore, s.gitSvc, s.templates)
	issueHandler := handlers.NewIssueHandler(s.issueStore, s.repoStore, s.userStore, s.templates)
	userHandler := handlers.NewUserHandler(s.userStore, s.tokenStore, s.cfg, s.templates)
	gitHTTPHandler := handlers.NewGitHTTPHandler(s.repoStore, s.userStore, s.tokenStore, s.gitSvc, s.cfg)

	r.Group(func(r chi.Router) {
		r.Get("/login", authHandler.LoginPage)
		r.Post("/login", authHandler.Login)
		r.Get("/register", authHandler.RegisterPage)
		r.Post("/register", authHandler.Register)
	})

	r.Group(func(r chi.Router) {
		r.Use(authMW.Required)
		r.Post("/logout", authHandler.Logout)

		r.Get("/", repoHandler.Dashboard)
		r.Get("/explore", repoHandler.Explore)
		r.Get("/new", repoHandler.NewRepoPage)
		r.Post("/new", repoHandler.CreateRepo)

		r.Get("/settings", userHandler.SettingsPage)
		r.Post("/settings", userHandler.UpdateProfile)
		r.Get("/settings/keys", userHandler.KeysPage)
		r.Post("/settings/keys", userHandler.AddKey)
		r.Post("/settings/keys/{id}/delete", userHandler.DeleteKey)
		r.Get("/settings/tokens", userHandler.TokensPage)
		r.Post("/settings/tokens", userHandler.CreateToken)
		r.Post("/settings/tokens/{id}/delete", userHandler.DeleteToken)
	})

	r.Route("/{owner}/{repo}.git", func(r chi.Router) {
		r.Get("/info/refs", gitHTTPHandler.InfoRefs)
		r.Post("/git-upload-pack", gitHTTPHandler.UploadPack)
		r.Post("/git-receive-pack", gitHTTPHandler.ReceivePack)
	})

	r.Route("/{owner}/{repo}", func(r chi.Router) {
		r.Use(authMW.Optional)

		r.Get("/", browseHandler.RepoRoot)
		r.Get("/tree/{ref}/*", browseHandler.Tree)
		r.Get("/tree/{ref}", browseHandler.Tree)
		r.Get("/blob/{ref}/*", browseHandler.Blob)
		r.Get("/raw/{ref}/*", browseHandler.Raw)
		r.Get("/commits/{ref}", browseHandler.Commits)
		r.Get("/commit/{sha}", browseHandler.Commit)

		r.Group(func(r chi.Router) {
			r.Use(authMW.Required)
			r.Get("/settings", repoHandler.SettingsPage)
			r.Post("/settings", repoHandler.UpdateSettings)
			r.Post("/settings/delete", repoHandler.DeleteRepo)
		})

		r.Route("/issues", func(r chi.Router) {
			r.Get("/", issueHandler.List)
			r.Group(func(r chi.Router) {
				r.Use(authMW.Required)
				r.Get("/new", issueHandler.NewPage)
				r.Post("/new", issueHandler.Create)
			})
			r.Get("/{number}", issueHandler.View)
			r.Group(func(r chi.Router) {
				r.Use(authMW.Required)
				r.Get("/{number}/edit", issueHandler.EditPage)
				r.Post("/{number}/edit", issueHandler.Update)
				r.Post("/{number}/close", issueHandler.Close)
				r.Post("/{number}/reopen", issueHandler.Reopen)
				r.Post("/{number}/comment", issueHandler.AddComment)
				r.Post("/{number}/labels", issueHandler.UpdateLabels)
				r.Post("/{number}/assignees", issueHandler.UpdateAssignees)
				r.Post("/{number}/react", issueHandler.AddReaction)
				r.Post("/comments/{commentID}/react", issueHandler.AddCommentReaction)
				r.Post("/comments/{commentID}/edit", issueHandler.EditComment)
				r.Post("/comments/{commentID}/delete", issueHandler.DeleteComment)
			})
		})

		r.Route("/labels", func(r chi.Router) {
			r.Use(authMW.Required)
			r.Get("/", issueHandler.Labels)
			r.Post("/", issueHandler.CreateLabel)
			r.Post("/{id}/edit", issueHandler.UpdateLabel)
			r.Post("/{id}/delete", issueHandler.DeleteLabel)
		})

		r.Route("/milestones", func(r chi.Router) {
			r.Use(authMW.Required)
			r.Get("/", issueHandler.Milestones)
			r.Post("/", issueHandler.CreateMilestone)
			r.Post("/{id}/edit", issueHandler.UpdateMilestone)
			r.Post("/{id}/close", issueHandler.CloseMilestone)
			r.Post("/{id}/reopen", issueHandler.ReopenMilestone)
			r.Post("/{id}/delete", issueHandler.DeleteMilestone)
		})
	})

	return r
}
