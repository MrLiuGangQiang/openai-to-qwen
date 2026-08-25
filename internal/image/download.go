package image

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// Fetcher downloads an image URL to bytes.
type Fetcher interface {
	Fetch(ctx context.Context, url string) ([]byte, error)
}

// Downloader downloads image URLs with bounded concurrency and a size cap.
type Downloader struct {
	client *http.Client
	max    int64
}

// NewDownloader creates a Downloader. client is the shared HTTP client.
func NewDownloader(client *http.Client, maxBytes int64) *Downloader {
	if maxBytes < 1 {
		maxBytes = 20 << 20
	}
	return &Downloader{client: client, max: maxBytes}
}

// Fetch downloads url into memory, enforcing the size cap.
func (d *Downloader) Fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "image/*")
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("download failed: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, d.max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > d.max {
		return nil, fmt.Errorf("image exceeds max size of %d bytes", d.max)
	}
	return data, nil
}

// fetchAll downloads urls concurrently with a bounded worker count.
func fetchAll(ctx context.Context, urls []string, f Fetcher, concurrency int) ([][]byte, error) {
	if concurrency < 1 {
		concurrency = 1
	}
	results := make([][]byte, len(urls))
	sem := make(chan struct{}, concurrency)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, len(urls))
	for i, u := range urls {
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
			defer func() { <-sem }()
			data, err := f.Fetch(ctx, u)
			if err != nil {
				errCh <- err
				cancel()
				return
			}
			results[i] = data
		}(i, u)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}
