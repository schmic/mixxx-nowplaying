package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/dhowden/tag"
	_ "modernc.org/sqlite"
)

const (
	defaultPollInterval = 5 * time.Second
	defaultListenAddr   = "0.0.0.0:9000"
)

var singlePage = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Mixxx Now Playing</title>
  <style>
    :root {
      color-scheme: dark;
      font-family: "Space Grotesk", "Inter", "Helvetica Neue", Arial, sans-serif;
      font-size: 16px;
      line-height: 1.4;
      color: #f5f7ff;
    }
    body {
      margin: 0;
      padding: 4px;
      min-height: 100vh;
      display: flex;
      align-items: flex-start;
      justify-content: flex-start;
      background: #05060c;
    }
    .card {
      padding: 1rem;
      background: transparent;
      box-shadow: 0 16px 36px rgba(0, 0, 0, 0.45);
      width: min(680px, 94vw);
      color: inherit;
      text-align: left;
      display: flex;
      align-items: center;
      gap: 1rem;
    }
    .cover {
      width: clamp(96px, 18vw, 144px);
      aspect-ratio: 1 / 1;
      flex: 0 0 auto;
      border-radius: 0.9rem;
      overflow: hidden;
      background:
        radial-gradient(circle at top left, rgba(255, 255, 255, 0.18), transparent 55%),
        linear-gradient(135deg, #1a1e33 0%, #0e111f 100%);
      box-shadow:
        0 12px 28px rgba(0, 0, 0, 0.35),
        inset 0 0 0 1px rgba(255, 255, 255, 0.08);
      position: relative;
      display: grid;
      place-items: center;
    }
    .cover img {
      width: 100%;
      height: 100%;
      display: block;
      object-fit: cover;
    }
    .cover.placeholder img {
      opacity: 0.92;
    }
    .meta {
      min-width: 0;
      display: flex;
      flex-direction: column;
      justify-content: center;
      gap: 0.75rem;
    }
    .artist {
      font-size: clamp(1.4rem, 2vw, 1.85rem);
      font-weight: 500;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      overflow-wrap: anywhere;
    }
    .title {
      font-size: clamp(1.1rem, 1.7vw, 1.55rem);
      letter-spacing: 0.02em;
      font-weight: 300;
      overflow-wrap: anywhere;
    }
    .fade-in {
      animation: fade 400ms ease-in-out;
    }
    @media (max-width: 520px) {
      .card {
        align-items: flex-start;
      }
      .cover {
        width: clamp(84px, 28vw, 120px);
      }
    }
    @keyframes fade {
      from { opacity: 0; transform: translateY(0.75rem); }
      to { opacity: 1; transform: translateY(0); }
    }
  </style>
</head>
<body>
  <main class="card">
    <div id="cover" class="cover placeholder">
      <img id="cover-image" alt="" src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 640 640'%3E%3Cdefs%3E%3ClinearGradient id='bg' x1='0%25' y1='0%25' x2='100%25' y2='100%25'%3E%3Cstop offset='0%25' stop-color='%231a1e33'/%3E%3Cstop offset='100%25' stop-color='%23090b14'/%3E%3C/linearGradient%3E%3CradialGradient id='glow' cx='20%25' cy='18%25' r='70%25'%3E%3Cstop offset='0%25' stop-color='rgba(255,255,255,0.22)'/%3E%3Cstop offset='100%25' stop-color='rgba(255,255,255,0)'/%3E%3C/radialGradient%3E%3C/defs%3E%3Crect width='640' height='640' rx='72' fill='url(%23bg)'/%3E%3Crect width='640' height='640' rx='72' fill='url(%23glow)'/%3E%3Ccircle cx='320' cy='320' r='164' fill='%23d5dcf6' fill-opacity='0.15'/%3E%3Ccircle cx='320' cy='320' r='114' fill='%23d5dcf6' fill-opacity='0.2'/%3E%3Ccircle cx='320' cy='320' r='28' fill='%23f5f7ff' fill-opacity='0.92'/%3E%3Cpath d='M215 184v198.5c16.9-18.4 42.2-29.5 70-29.5 51.4 0 93 37.5 93 83.8S336.4 521 285 521s-93-37.5-93-83.8V184h23Zm233 0v128.5c16.9-18.4 42.2-29.5 70-29.5 51.4 0 93 37.5 93 83.8S569.4 451 518 451s-93-37.5-93-83.8V184h23Z' fill='%23f5f7ff' fill-opacity='0.92'/%3E%3C/svg%3E">
    </div>
    <div class="meta">
      <div id="artist" class="artist fade-in">Loading artist…</div>
      <div id="title" class="title fade-in">Loading title…</div>
    </div>
  </main>
  <script>
    const artistEl = document.getElementById('artist');
    const titleEl = document.getElementById('title');
    const coverEl = document.getElementById('cover');
    const coverImageEl = document.getElementById('cover-image');
    const placeholderCoverUrl = coverImageEl.getAttribute('src');

    function updateCover(coverUrl) {
      if (!coverUrl) {
        coverImageEl.dataset.src = '';
        coverImageEl.src = placeholderCoverUrl;
        coverEl.classList.add('placeholder');
        return;
      }
      if (coverImageEl.dataset.src === coverUrl) {
        return;
      }
      coverImageEl.dataset.src = coverUrl;
      coverImageEl.src = coverUrl;
    }

    coverImageEl.addEventListener('load', () => {
      if (coverImageEl.currentSrc === placeholderCoverUrl) {
        coverEl.classList.add('placeholder');
        return;
      }
      coverEl.classList.remove('placeholder');
    });

    coverImageEl.addEventListener('error', () => {
      coverImageEl.dataset.src = '';
      if (coverImageEl.currentSrc !== placeholderCoverUrl) {
        coverImageEl.src = placeholderCoverUrl;
      }
      coverEl.classList.add('placeholder');
    });

    async function refreshNowPlaying() {
      try {
        const response = await fetch('/api/now', {cache: 'no-store'});
        if (!response.ok) {
          throw new Error('Request failed');
        }
        const payload = await response.json();
        const artist = payload.artist || 'Unknown Artist';
        const title = payload.title || 'Unknown Title';
        updateCover(payload.coverUrl || '');

        if (artistEl.textContent !== artist) {
          artistEl.textContent = artist;
          artistEl.classList.remove('fade-in');
          void artistEl.offsetWidth;
          artistEl.classList.add('fade-in');
        }
        if (titleEl.textContent !== title) {
          titleEl.textContent = title;
          titleEl.classList.remove('fade-in');
          void titleEl.offsetWidth;
          titleEl.classList.add('fade-in');
        }
      } catch (error) {
        console.error(error);
      }
    }

    refreshNowPlaying();
    setInterval(refreshNowPlaying, 5000);
  </script>
</body>
</html>`))

type track struct {
	ID     int64
	Artist string
	Title  string
	Path   string
}

type trackSnapshot struct {
	Track       track
	RetrievedAt time.Time
	Err         error
}

type coverSnapshot struct {
	TrackID     int64
	Path        string
	Image       []byte
	ContentType string
	Err         error
}

type trackStore struct {
	db        *sql.DB
	mu        sync.RWMutex
	snapshot  trackSnapshot
	cover     coverSnapshot
	queryOnce func(context.Context, *sql.DB) (track, error)
}

func newTrackStore(db *sql.DB) *trackStore {
	return &trackStore{
		db:        db,
		queryOnce: queryLatestTrack,
	}
}

func (s *trackStore) startPolling(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.poll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.poll(ctx)
		}
	}
}

func (s *trackStore) poll(ctx context.Context) {
	track, err := s.queryOnce(ctx, s.db)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot = trackSnapshot{
		Track:       track,
		RetrievedAt: time.Now(),
		Err:         err,
	}
	if err != nil {
		log.Printf("failed to fetch track: %v", err)
	}
}

func (s *trackStore) current() trackSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot
}

func (s *trackStore) coverArt(ctx context.Context, current track) ([]byte, string, error) {
	if current.ID == 0 || current.Path == "" {
		return nil, "", errNoCoverArt
	}

	s.mu.RLock()
	cached := s.cover
	s.mu.RUnlock()
	if cached.TrackID == current.ID && cached.Path == current.Path {
		if cached.Err != nil {
			return nil, "", cached.Err
		}
		return append([]byte(nil), cached.Image...), cached.ContentType, nil
	}

	image, contentType, err := extractCoverArt(ctx, current.Path)

	s.mu.Lock()
	s.cover = coverSnapshot{
		TrackID:     current.ID,
		Path:        current.Path,
		Image:       append([]byte(nil), image...),
		ContentType: contentType,
		Err:         err,
	}
	s.mu.Unlock()

	if err != nil {
		return nil, "", err
	}
	return image, contentType, nil
}

var errNoCoverArt = errors.New("no cover art available")

func queryLatestTrack(ctx context.Context, db *sql.DB) (track, error) {
	const statement = `
	SELECT
		l.id,
		COALESCE(NULLIF(l.artist, ''), 'Unknown Artist') AS artist,
		COALESCE(NULLIF(l.title, ''), 'Unknown Title') AS title,
		COALESCE(tl.directory, '') AS directory,
		COALESCE(tl.filename, '') AS filename
	FROM PlaylistTracks pt
	JOIN library l ON l.id = pt.track_id
	LEFT JOIN track_locations tl ON tl.id = l.location
	WHERE pt.id = (SELECT MAX(id) FROM PlaylistTracks)
	LIMIT 1;
	`
	var (
		id        int64
		artist    string
		title     string
		directory string
		filename  string
	)

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	err := db.QueryRowContext(ctx, statement).Scan(&id, &artist, &title, &directory, &filename)
	if errors.Is(err, sql.ErrNoRows) {
		return track{}, nil
	}
	if err != nil {
		return track{}, err
	}

	var path string
	if directory != "" && filename != "" {
		path = filepath.Join(directory, filename)
	}

	return track{ID: id, Artist: artist, Title: title, Path: path}, nil
}

func indexHandler(store *trackStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := singlePage.Execute(w, nil); err != nil {
			log.Printf("render index: %v", err)
		}
	}
}

func apiHandler(store *trackStore) http.HandlerFunc {
	type response struct {
		TrackID     int64     `json:"trackId"`
		Artist      string    `json:"artist"`
		Title       string    `json:"title"`
		CoverURL    string    `json:"coverUrl,omitempty"`
		RetrievedAt time.Time `json:"retrievedAt,omitempty"`
		Error       string    `json:"error,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		snapshot := store.current()
		payload := response{
			TrackID:     snapshot.Track.ID,
			Artist:      snapshot.Track.Artist,
			Title:       snapshot.Track.Title,
			RetrievedAt: snapshot.RetrievedAt,
		}
		if snapshot.Track.ID != 0 && snapshot.Track.Path != "" {
			payload.CoverURL = fmt.Sprintf("/api/cover?track=%d", snapshot.Track.ID)
		}
		if snapshot.Err != nil {
			payload.Error = snapshot.Err.Error()
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			log.Printf("encode api response: %v", err)
		}
	}
}

func coverHandler(store *trackStore, logger *log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		trackID, err := strconv.ParseInt(r.URL.Query().Get("track"), 10, 64)
		if err != nil || trackID <= 0 {
			http.Error(w, "missing track id", http.StatusBadRequest)
			return
		}

		snapshot := store.current()
		if snapshot.Track.ID == 0 || snapshot.Track.ID != trackID {
			http.NotFound(w, r)
			return
		}

		image, contentType, err := store.coverArt(r.Context(), snapshot.Track)
		if err != nil {
			if !errors.Is(err, errNoCoverArt) {
				logger.Printf("load cover art for track %d: %v", trackID, err)
			}
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(image); err != nil {
			logger.Printf("write cover art response for track %d: %v", trackID, err)
		}
	}
}

func extractCoverArt(ctx context.Context, trackPath string) ([]byte, string, error) {
	if trackPath == "" {
		return nil, "", errNoCoverArt
	}

	if err := ctx.Err(); err != nil {
		return nil, "", err
	}

	file, err := os.Open(trackPath)
	if err != nil {
		return nil, "", fmt.Errorf("open track for cover art: %w", err)
	}
	defer file.Close()

	metadata, err := tag.ReadFrom(file)
	if err != nil {
		if errors.Is(err, tag.ErrNoTagsFound) {
			return nil, "", errNoCoverArt
		}
		return nil, "", fmt.Errorf("read track metadata: %w", err)
	}

	picture := metadata.Picture()
	if picture == nil || len(picture.Data) == 0 {
		return nil, "", errNoCoverArt
	}

	contentType := picture.MIMEType
	if contentType == "" {
		contentType = http.DetectContentType(picture.Data)
	}
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = "image/jpeg"
	}

	return picture.Data, contentType, nil
}

func main() {
	logger := log.New(os.Stdout, "[mixxx-nowplaying] ", log.LstdFlags|log.Lmsgprefix)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbPath := os.Getenv("MIXXX_DB_PATH")
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			logger.Fatalf("resolve home directory: %v", err)
		}
		dbPath = filepath.Join(home, ".mixxx", "mixxxdb.sqlite")
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		logger.Fatalf("open database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.PingContext(ctx); err != nil {
		logger.Fatalf("ping database: %v", err)
	}

	store := newTrackStore(db)
	go store.startPolling(ctx, defaultPollInterval)

	mux := http.NewServeMux()
	mux.Handle("/", indexHandler(store))
	mux.Handle("/api/now", apiHandler(store))
	mux.Handle("/api/cover", coverHandler(store, logger))

	server := &http.Server{
		Addr:         defaultListenAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  2 * time.Minute,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Printf("listening on %s", defaultListenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
		close(serverErrors)
	}()

	select {
	case <-ctx.Done():
		logger.Println("shutting down...")
	case err := <-serverErrors:
		if err != nil {
			logger.Fatalf("server error: %v", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Printf("graceful shutdown failed: %v", err)
	}
}
