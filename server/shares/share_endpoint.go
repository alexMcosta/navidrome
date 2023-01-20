package shares

import (
	"errors"
	"net/http"
	"path"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server"
	"github.com/navidrome/navidrome/ui"
)

type Router struct {
	http.Handler
	ds            model.DataStore
	share         core.Share
	assetsHandler http.Handler
	streamer      core.MediaStreamer
}

func New(ds model.DataStore, share core.Share) *Router {
	p := &Router{ds: ds, share: share}
	shareRoot := path.Join(conf.Server.BaseURL, consts.URLPathShares)
	p.assetsHandler = http.StripPrefix(shareRoot, http.FileServer(http.FS(ui.BuildAssets())))
	p.Handler = p.routes()

	return p
}

func (p *Router) routes() http.Handler {
	r := chi.NewRouter()

	r.Group(func(r chi.Router) {
		r.Use(server.URLParamsMiddleware)
		r.HandleFunc("/{id}", p.handleShares)
		r.Handle("/*", p.assetsHandler)
	})
	return r
}

func (p *Router) handleShares(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get(":id")
	if id == "" {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	// If requested file is a UI asset, just serve it
	_, err := ui.BuildAssets().Open(id)
	if err == nil {
		p.assetsHandler.ServeHTTP(w, r)
		return
	}

	// If it is not, consider it a share ID
	s, err := p.share.Load(r.Context(), id)
	switch {
	case errors.Is(err, model.ErrNotFound):
		log.Error(r, "Share not found", "id", id, err)
		http.Error(w, "Share not found", http.StatusNotFound)
	case err != nil:
		log.Error(r, "Error retrieving share", "id", id, err)
		http.Error(w, "Error retrieving share", http.StatusInternalServerError)
	}
	if err != nil {
		return
	}

	s = p.mapShareInfo(s)
	server.IndexWithShare(p.ds, ui.BuildAssets(), s)(w, r)
}

func (p *Router) mapShareInfo(s *model.Share) *model.Share {
	mapped := &model.Share{
		Description: s.Description,
		Tracks:      s.Tracks,
	}
	for i := range s.Tracks {
		claims := map[string]any{"id": s.Tracks[i].ID}
		if s.Format != "" {
			claims["f"] = s.Format
		}
		if s.MaxBitRate != 0 {
			claims["b"] = s.MaxBitRate
		}
		id, _ := auth.CreateExpiringPublicToken(*s.ExpiresAt, claims)
		mapped.Tracks[i].ID = id
	}
	return mapped
}
