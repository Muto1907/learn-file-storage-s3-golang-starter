package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	reader := http.MaxBytesReader(w, r.Body, 1<<30)
	r.Body = reader
	videoIdString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIdString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}
	userId, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Video not found", err)
		return
	}
	if video.UserID != userId {
		respondWithError(w, http.StatusUnauthorized, "User is not video owner", nil)
		return
	}
	videoFile, videoHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get File from Form", err)
		return
	}
	defer videoFile.Close()
	mimeType, _, err := mime.ParseMediaType(videoHeader.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't parse Media Type", err)
		return
	}
	if mimeType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Wrong media type for videoupload, expected: mp4", err)
		return
	}
	localFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't Create Temp file", err)
		return
	}
	defer os.Remove(localFile.Name())
	defer localFile.Close()
	_, err = io.Copy(localFile, videoFile)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't copy video to local file", err)
		return
	}
	_, err = localFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't reset Pointer of local File", err)
		return
	}
	aspectRatio, err := getVideoAspectRatio(localFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video Aspect Ratio", err)
		return
	}
	randKey := make([]byte, 32)
	n, err := rand.Read(randKey)
	if n != 32 {
		respondWithError(w, http.StatusInternalServerError, "Couldn't initialize random filekey name", err)
		return
	}
	fileKey := hex.EncodeToString(randKey) + ".mp4"
	if aspectRatio == "16:9" {
		fileKey = "landscape/" + fileKey
	} else if aspectRatio == "9:16" {
		fileKey = "portrait/" + fileKey
	} else {
		fileKey = "other/" + fileKey
	}

	_, err = cfg.sClient.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &fileKey,
		Body:        localFile,
		ContentType: &mimeType,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't put Object", err)
		return
	}
	URL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, fileKey)
	video.VideoURL = &URL
	video.UpdatedAt = time.Now()
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video in db", err)
		return
	}
	respondWithJSON(w, http.StatusOK, video)

}
