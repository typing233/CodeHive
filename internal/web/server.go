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
	"github.com/codehive/codehive/internal/webhook"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	cfg              *config.Config
	userStore        *store.UserStore
	repoStore        *store.RepoStore
	issueStore       *store.IssueStore
	sessionStore     *store.SessionStore
	tokenStore       *store.TokenStore
	prStore          *store.PRStore
	orgStore         *store.OrgStore
	auditStore       *store.AuditStore
	webhookStore     *store.WebhookStore
	packageStore     *store.PackageStore
	gitSvc           *gitbackend.Service
	webhookDispatch  *webhook.Dispatcher
	renderer         *Renderer
}

func NewServer(cfg *config.Config, us *store.UserStore, rs *store.RepoStore, is *store.IssueStore,
	ss *store.SessionStore, ts *store.TokenStore, ps *store.PRStore, os *store.OrgStore,
	as *store.AuditStore, ws *store.WebhookStore, pks *store.PackageStore,
	gs *gitbackend.Service, wd *webhook.Dispatcher) *Server {
	s := &Server{
		cfg:             cfg,
		userStore:       us,
		repoStore:       rs,
		issueStore:      is,
		sessionStore:    ss,
		tokenStore:      ts,
		prStore:         ps,
		orgStore:        os,
		auditStore:      as,
		webhookStore:    ws,
		packageStore:    pks,
		gitSvc:         gs,
		webhookDispatch: wd,
	}
	s.renderer = NewRenderer()
	return s
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(compressMiddleware)

	authMW := middleware.NewAuthMiddleware(s.sessionStore, s.userStore)

	authHandler := handlers.NewAuthHandler(s.userStore, s.sessionStore, s.cfg, s.renderer)
	repoHandler := handlers.NewRepoHandler(s.repoStore, s.userStore, s.auditStore, s.gitSvc, s.cfg, s.renderer)
	browseHandler := handlers.NewBrowseHandler(s.repoStore, s.gitSvc, s.renderer)
	issueHandler := handlers.NewIssueHandler(s.issueStore, s.repoStore, s.userStore, s.renderer)
	userHandler := handlers.NewUserHandler(s.userStore, s.tokenStore, s.cfg, s.renderer)
	gitHTTPHandler := handlers.NewGitHTTPHandler(s.repoStore, s.userStore, s.tokenStore, s.gitSvc, s.webhookDispatch, s.cfg)
	prHandler := handlers.NewPRHandler(s.prStore, s.repoStore, s.issueStore, s.userStore, s.auditStore, s.gitSvc, s.webhookDispatch, s.renderer)
	orgHandler := handlers.NewOrgHandler(s.orgStore, s.repoStore, s.userStore, s.tokenStore, s.auditStore, s.cfg, s.renderer)
	webhookHandler := handlers.NewWebhookHandler(s.webhookStore, s.repoStore, s.webhookDispatch, s.renderer)
	auditHandler := handlers.NewAuditHandler(s.auditStore, s.renderer)
	npmHandler := handlers.NewNPMHandler(s.packageStore, s.userStore, s.tokenStore, s.auditStore, s.webhookDispatch, s.cfg)
	dockerHandler := handlers.NewDockerHandler(s.packageStore, s.userStore, s.tokenStore, s.auditStore, s.webhookDispatch, s.cfg)

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("internal/web/static"))))

	// Public auth routes
	r.Group(func(r chi.Router) {
		r.Get("/login", authHandler.LoginPage)
		r.Post("/login", authHandler.Login)
		r.Get("/register", authHandler.RegisterPage)
		r.Post("/register", authHandler.Register)
	})

	// Authenticated routes
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

		// Organization routes
		r.Get("/orgs/new", orgHandler.NewPage)
		r.Post("/orgs/new", orgHandler.Create)

		// Admin audit log
		r.Get("/admin/audit", auditHandler.SiteAudit)
	})

	// Docker Registry API (v2)
	r.Route("/v2", func(r chi.Router) {
		r.Get("/", dockerHandler.VersionCheck)
		r.Get("/{name}/manifests/{reference}", dockerHandler.GetManifest)
		r.Put("/{name}/manifests/{reference}", dockerHandler.PutManifest)
		r.Delete("/{name}/manifests/{reference}", dockerHandler.DeleteManifest)
		r.Head("/{name}/blobs/{digest}", dockerHandler.HeadBlob)
		r.Get("/{name}/blobs/{digest}", dockerHandler.GetBlob)
		r.Post("/{name}/blobs/uploads/", dockerHandler.InitBlobUpload)
		r.Patch("/{name}/blobs/uploads/{uuid}", dockerHandler.ChunkBlobUpload)
		r.Put("/{name}/blobs/uploads/{uuid}", dockerHandler.CompleteBlobUpload)
		r.Get("/{name}/tags/list", dockerHandler.ListTags)
	})

	// NPM Registry API
	r.Route("/api/packages/npm", func(r chi.Router) {
		r.Get("/@{scope}/{name}", npmHandler.GetMetadata)
		r.Get("/{name}", npmHandler.GetMetadata)
		r.Get("/@{scope}/{name}/-/{tarball}", npmHandler.DownloadTarball)
		r.Get("/{name}/-/{tarball}", npmHandler.DownloadTarball)
		r.Put("/@{scope}/{name}", npmHandler.Publish)
		r.Put("/{name}", npmHandler.Publish)
		r.Delete("/@{scope}/{name}/-/{tarball}/-rev/{rev}", npmHandler.Unpublish)
		r.Delete("/{name}/-/{tarball}/-rev/{rev}", npmHandler.Unpublish)
	})

	// Git Smart HTTP
	r.Route("/{owner}/{repo}.git", func(r chi.Router) {
		r.Get("/info/refs", gitHTTPHandler.InfoRefs)
		r.Post("/git-upload-pack", gitHTTPHandler.UploadPack)
		r.Post("/git-receive-pack", gitHTTPHandler.ReceivePack)
	})

	// Organization routes (dynamic namespace)
	r.Route("/orgs/{org}", func(r chi.Router) {
		r.Use(authMW.Optional)
		r.Get("/", orgHandler.Profile)
		r.Get("/members", orgHandler.Members)
		r.Get("/teams", orgHandler.Teams)
		r.Group(func(r chi.Router) {
			r.Use(authMW.Required)
			r.Post("/members/add", orgHandler.AddMember)
			r.Post("/members/{userID}/remove", orgHandler.RemoveMember)
			r.Post("/teams/new", orgHandler.CreateTeam)
			r.Post("/teams/{teamID}/edit", orgHandler.UpdateTeam)
			r.Post("/teams/{teamID}/delete", orgHandler.DeleteTeam)
			r.Post("/teams/{teamID}/members", orgHandler.UpdateTeamMembers)
			r.Post("/teams/{teamID}/repos", orgHandler.UpdateTeamRepos)
			r.Get("/settings", orgHandler.Settings)
			r.Post("/settings", orgHandler.UpdateSettings)
			r.Post("/settings/delete", orgHandler.Delete)
			r.Get("/settings/tokens", orgHandler.TokensPage)
			r.Post("/settings/tokens", orgHandler.CreateToken)
			r.Post("/settings/tokens/{id}/delete", orgHandler.DeleteToken)
			r.Get("/settings/audit", auditHandler.OrgAudit)
		})
	})

	// Repository routes
	r.Route("/{owner}/{repo}", func(r chi.Router) {
		r.Use(authMW.Optional)

		r.Get("/", browseHandler.RepoRoot)
		r.Get("/tree/{ref}/*", browseHandler.Tree)
		r.Get("/tree/{ref}", browseHandler.Tree)
		r.Get("/blob/{ref}/*", browseHandler.Blob)
		r.Get("/raw/{ref}/*", browseHandler.Raw)
		r.Get("/commits/{ref}", browseHandler.Commits)
		r.Get("/commit/{sha}", browseHandler.Commit)

		// Repository settings
		r.Group(func(r chi.Router) {
			r.Use(authMW.Required)
			r.Get("/settings", repoHandler.SettingsPage)
			r.Post("/settings", repoHandler.UpdateSettings)
			r.Post("/settings/delete", repoHandler.DeleteRepo)

			// Webhooks (inside repo settings)
			r.Get("/settings/webhooks", webhookHandler.List)
			r.Post("/settings/webhooks", webhookHandler.Create)
			r.Get("/settings/webhooks/{id}", webhookHandler.View)
			r.Post("/settings/webhooks/{id}/edit", webhookHandler.Update)
			r.Post("/settings/webhooks/{id}/delete", webhookHandler.Delete)
			r.Post("/settings/webhooks/{id}/test", webhookHandler.TestDeliver)

			// Repo audit log
			r.Get("/settings/audit", auditHandler.RepoAudit)
		})

		// Pull Requests
		r.Route("/pulls", func(r chi.Router) {
			r.Get("/", prHandler.List)
			r.Group(func(r chi.Router) {
				r.Use(authMW.Required)
				r.Get("/new", prHandler.NewPage)
				r.Post("/new", prHandler.Create)
			})
			r.Get("/{number}", prHandler.View)
			r.Get("/{number}/diff", prHandler.Diff)
			r.Get("/{number}/commits", prHandler.Commits)
			r.Group(func(r chi.Router) {
				r.Use(authMW.Required)
				r.Post("/{number}/comment", prHandler.AddComment)
				r.Post("/{number}/review", prHandler.SubmitReview)
				r.Post("/{number}/merge", prHandler.Merge)
				r.Post("/{number}/close", prHandler.Close)
				r.Post("/{number}/reopen", prHandler.Reopen)
				r.Post("/{number}/labels", prHandler.UpdateLabels)
				r.Post("/{number}/assignees", prHandler.UpdateAssignees)
				r.Post("/comments/{commentID}/edit", prHandler.EditComment)
				r.Post("/comments/{commentID}/delete", prHandler.DeleteComment)
			})
		})

		// Issues
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

		// Labels
		r.Route("/labels", func(r chi.Router) {
			r.Use(authMW.Required)
			r.Get("/", issueHandler.Labels)
			r.Post("/", issueHandler.CreateLabel)
			r.Post("/{id}/edit", issueHandler.UpdateLabel)
			r.Post("/{id}/delete", issueHandler.DeleteLabel)
		})

		// Milestones
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

func compressMiddleware(next http.Handler) http.Handler {
	compressor := chimw.NewCompressor(5)
	compressed := compressor.Handler(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/info/refs") ||
			strings.HasSuffix(r.URL.Path, "/git-upload-pack") ||
			strings.HasSuffix(r.URL.Path, "/git-receive-pack") {
			next.ServeHTTP(w, r)
			return
		}
		compressed.ServeHTTP(w, r)
	})
}

// Renderer holds pre-parsed templates, one per page.
type Renderer struct {
	templates map[string]*template.Template
	funcMap   template.FuncMap
}

func NewRenderer() *Renderer {
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
		"deref": func(p interface{}) interface{} {
			if p == nil {
				return nil
			}
			if ptr, ok := p.(*int64); ok {
				if ptr == nil {
					return int64(0)
				}
				return *ptr
			}
			return p
		},
	}

	r := &Renderer{
		templates: make(map[string]*template.Template),
		funcMap:   funcMap,
	}

	layoutFiles := []string{
		"internal/web/templates/layout/base.html",
	}

	var pageFiles []string
	filepath.WalkDir("internal/web/templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".html") && !strings.Contains(path, "layout/") {
			pageFiles = append(pageFiles, path)
		}
		return nil
	})

	for _, pageFile := range pageFiles {
		name := filepath.Base(pageFile)
		name = strings.TrimSuffix(name, ".html")

		t := template.New("").Funcs(funcMap)
		t, err := t.ParseFiles(append(layoutFiles, pageFile)...)
		if err != nil {
			log.Fatalf("Failed to parse template %s: %v", pageFile, err)
		}
		r.templates[name] = t
	}

	log.Printf("Loaded %d templates", len(r.templates))
	return r
}

func (r *Renderer) Render(w http.ResponseWriter, name string, data interface{}) {
	t, ok := r.templates[name]
	if !ok {
		log.Printf("Template not found: %s", name)
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	err := t.ExecuteTemplate(w, name+".html", data)
	if err != nil {
		log.Printf("Template render error (%s): %v", name, err)
		http.Error(w, "Render error", http.StatusInternalServerError)
	}
}
