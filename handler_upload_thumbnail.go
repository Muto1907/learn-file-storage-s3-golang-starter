package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	thumbnailMediaType := header.Header.Get("Content-Type")
	if thumbnailMediaType == "" {
		respondWithError(w, http.StatusBadRequest, "Missing Content-Type Header in request", err)
		return
	}

	mimeType, _, err := mime.ParseMediaType(thumbnailMediaType)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't parse MediaType", err)
		return
	}
	if mimeType != "image/jpeg" && mimeType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Unsupported Mime Type for thumbnail in Content-Type Header", err)
		return
	}
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't find video", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized access to video", err)
		return
	}
	var fileNameWithExtension string
	extensionSplit := strings.Split(thumbnailMediaType, "/")
	thumbnailFileName := make([]byte, 32)
	n, err := rand.Read(thumbnailFileName)
	if n != 32 {
		respondWithError(w, http.StatusInternalServerError, "Couldn't initialize random thumbnailname", err)
		return
	}
	encodedThumbnailFileName := base64.RawURLEncoding.EncodeToString(thumbnailFileName)
	if len(extensionSplit) != 2 {
		fileNameWithExtension = encodedThumbnailFileName + ".bin"
	} else {
		fileNameWithExtension = encodedThumbnailFileName + "." + extensionSplit[1]
	}
	path := filepath.Join(cfg.assetsRoot, fileNameWithExtension)
	thumbnailFile, err := os.Create(path)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't Create Thumbnail file", err)
		return
	}
	_, err = io.Copy(thumbnailFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't copy thumbnail to assets", err)
		return
	}
	dataURL := fmt.Sprintf("http://localhost:%s/%s", cfg.port, path)
	video.ThumbnailURL = &dataURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update videothumbnail URL", err)
		return
	}
	respondWithJSON(w, http.StatusOK, video)
}
