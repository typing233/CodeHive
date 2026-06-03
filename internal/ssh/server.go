package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/codehive/codehive/internal/config"
	"github.com/codehive/codehive/internal/gitbackend"
	"github.com/codehive/codehive/internal/store"
	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

type Server struct {
	cfg       *config.Config
	userStore *store.UserStore
	repoStore *store.RepoStore
	gitSvc    *gitbackend.Service
	sshServer *ssh.Server
}

func NewServer(cfg *config.Config, us *store.UserStore, rs *store.RepoStore, gs *gitbackend.Service) *Server {
	s := &Server{
		cfg:       cfg,
		userStore: us,
		repoStore: rs,
		gitSvc:    gs,
	}

	s.sshServer = &ssh.Server{
		Addr:             fmt.Sprintf(":%d", cfg.SSH.Port),
		PublicKeyHandler: s.publicKeyHandler,
		Handler:          s.sessionHandler,
	}

	hostKey, err := s.loadOrGenerateHostKey()
	if err != nil {
		log.Fatalf("SSH host key error: %v", err)
	}
	s.sshServer.AddHostKey(hostKey)

	return s
}

func (s *Server) ListenAndServe() error {
	return s.sshServer.ListenAndServe()
}

func (s *Server) Close() error {
	return s.sshServer.Close()
}

func (s *Server) publicKeyHandler(ctx ssh.Context, key ssh.PublicKey) bool {
	fp := store.ComputeSSHFingerprint(key)
	user, err := s.userStore.GetUserByFingerprint(context.Background(), fp)
	if err != nil {
		return false
	}
	ctx.SetValue("user_id", user.ID)
	ctx.SetValue("username", user.Username)
	return true
}

func (s *Server) sessionHandler(sess ssh.Session) {
	cmd := sess.RawCommand()
	if cmd == "" {
		fmt.Fprintf(sess, "Hi! You've successfully authenticated, but CodeHive does not provide shell access.\n")
		sess.Exit(0)
		return
	}

	parts := strings.Fields(cmd)
	if len(parts) != 2 {
		fmt.Fprintf(sess.Stderr(), "Invalid command\n")
		sess.Exit(1)
		return
	}

	gitCmd := parts[0]
	if gitCmd != "git-upload-pack" && gitCmd != "git-receive-pack" {
		fmt.Fprintf(sess.Stderr(), "Unknown command: %s\n", gitCmd)
		sess.Exit(1)
		return
	}

	repoPath := strings.Trim(parts[1], "'\"")
	repoPath = strings.TrimPrefix(repoPath, "/")
	repoPath = strings.TrimSuffix(repoPath, ".git")

	pathParts := strings.SplitN(repoPath, "/", 2)
	if len(pathParts) != 2 {
		fmt.Fprintf(sess.Stderr(), "Invalid repository path\n")
		sess.Exit(1)
		return
	}

	owner := pathParts[0]
	repoName := pathParts[1]
	userID := sess.Context().Value("user_id").(int64)

	repo, err := s.repoStore.GetByOwnerAndName(context.Background(), owner, repoName)
	if err != nil {
		fmt.Fprintf(sess.Stderr(), "Repository not found: %s/%s\n", owner, repoName)
		sess.Exit(1)
		return
	}

	if gitCmd == "git-receive-pack" {
		hasAccess, _ := s.repoStore.HasAccess(context.Background(), repo.ID, userID, "write")
		if !hasAccess {
			fmt.Fprintf(sess.Stderr(), "Permission denied (no write access)\n")
			sess.Exit(1)
			return
		}
	} else {
		if repo.IsPrivate {
			hasAccess, _ := s.repoStore.HasAccess(context.Background(), repo.ID, userID, "read")
			if !hasAccess {
				fmt.Fprintf(sess.Stderr(), "Permission denied\n")
				sess.Exit(1)
				return
			}
		}
	}

	absRepoPath := s.gitSvc.AbsPath(repo.DiskPath)
	execCmd := exec.Command(gitCmd, absRepoPath)
	execCmd.Stdin = sess
	execCmd.Stdout = sess
	execCmd.Stderr = sess.Stderr()

	if err := execCmd.Run(); err != nil {
		sess.Exit(1)
		return
	}
	sess.Exit(0)
}

func (s *Server) loadOrGenerateHostKey() (gossh.Signer, error) {
	keyPath := s.cfg.SSH.HostKeyPath
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(keyPath)
	if err == nil {
		return gossh.ParsePrivateKey(data)
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	privBytes, err := gossh.MarshalPrivateKey(priv, "")
	if err != nil {
		return nil, err
	}

	pemData := pem.EncodeToMemory(privBytes)
	if err := os.WriteFile(keyPath, pemData, 0600); err != nil {
		return nil, err
	}

	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		return nil, err
	}

	log.Printf("Generated new SSH host key at %s", keyPath)
	return signer.(gossh.Signer), nil
}

func (s *Server) Address() string {
	return s.sshServer.Addr
}
