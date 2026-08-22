package orchestrator

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// RunBackup zips the tenant's data directory and uploads it to S3 (or local mock if no bucket).
func RunBackup(dataDir string, tenantID string) error {
	log.Printf("Starting backup for tenant %s from %s", tenantID, dataDir)

	timestamp := time.Now().Format("20060102_150405")
	zipFileName := fmt.Sprintf("backup_%s_%s.zip", tenantID, timestamp)
	localZipPath := filepath.Join(os.TempDir(), zipFileName)

	if err := zipDirectory(dataDir, localZipPath); err != nil {
		return fmt.Errorf("failed to zip data directory: %w", err)
	}

	bucket := os.Getenv("AWS_S3_BUCKET")
	if bucket == "" {
		log.Printf("No AWS_S3_BUCKET configured. Saving backup locally to %s", localZipPath)
		// For MVP/Mock mode, just leave it in TempDir or move to a local backup dir
		mockDir := "./data/backups"
		os.MkdirAll(mockDir, 0755)
		dest := filepath.Join(mockDir, zipFileName)
		if err := copyFile(localZipPath, dest); err != nil {
			return err
		}
		log.Printf("Mock S3 Backup completed successfully: %s", dest)
		return nil
	}

	// Real S3 Upload
	log.Printf("Uploading %s to S3 bucket %s...", zipFileName, bucket)
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg)
	file, err := os.Open(localZipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip file for upload: %w", err)
	}
	defer file.Close()

	_, err = client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &zipFileName,
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	log.Printf("S3 Backup uploaded successfully: s3://%s/%s", bucket, zipFileName)
	os.Remove(localZipPath) // cleanup
	return nil
}

func zipDirectory(source, target string) error {
	zipfile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer zipfile.Close()

	archive := zip.NewWriter(zipfile)
	defer archive.Close()

	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		header.Name, err = filepath.Rel(filepath.Dir(source), path)
		if err != nil {
			return err
		}
		
		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(writer, file)
		return err
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
