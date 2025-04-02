package internal

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// Resource is any resource that needs to be managed and cleaned up
type Resource interface {
	Close() error
}

// ResourceGroup manages a group of resources that need to be cleaned up together
type ResourceGroup struct {
	resources []Resource
	mutex     sync.Mutex
	closed    bool
}

// NewResourceGroup creates a new resource group
func NewResourceGroup() *ResourceGroup {
	return &ResourceGroup{
		resources: make([]Resource, 0),
	}
}

// Add adds a resource to the group
func (rg *ResourceGroup) Add(r Resource) {
	if r == nil {
		return
	}
	
	rg.mutex.Lock()
	defer rg.mutex.Unlock()
	
	if rg.closed {
		// Group is already closed, close the resource immediately
		if err := r.Close(); err != nil {
			log.Printf("Error closing resource: %v", err)
		}
		return
	}
	
	rg.resources = append(rg.resources, r)
}

// Close closes all resources in the group
func (rg *ResourceGroup) Close() error {
	rg.mutex.Lock()
	defer rg.mutex.Unlock()
	
	if rg.closed {
		return nil
	}
	
	rg.closed = true
	
	var lastErr error
	for _, r := range rg.resources {
		if err := r.Close(); err != nil {
			log.Printf("Error closing resource: %v", err)
			lastErr = err
		}
	}
	
	// Clear resources after closing them
	rg.resources = nil
	
	return lastErr
}

// ResourceWithTimeout wraps a resource with a timeout
type ResourceWithTimeout struct {
	Resource
	timeout time.Duration
}

// NewResourceWithTimeout creates a new resource with timeout
func NewResourceWithTimeout(r Resource, timeout time.Duration) *ResourceWithTimeout {
	return &ResourceWithTimeout{
		Resource: r,
		timeout:  timeout,
	}
}

// Close closes the resource with a timeout
func (rwt *ResourceWithTimeout) Close() error {
	ch := make(chan error, 1)
	
	go func() {
		ch <- rwt.Resource.Close()
	}()
	
	select {
	case err := <-ch:
		return err
	case <-time.After(rwt.timeout):
		return fmt.Errorf("resource close timeout: %w", context.DeadlineExceeded)
	}
}

// CloseQuietly closes a resource without returning an error
func CloseQuietly(r io.Closer) {
	if r != nil {
		_ = r.Close()
	}
}

// CloseWithLogging closes a resource and logs any error
func CloseWithLogging(r io.Closer, name string) {
	if r == nil {
		return
	}
	
	if err := r.Close(); err != nil {
		log.Printf("Error closing %s: %v", name, err)
		IncrementErrorMetric("resource_close_error")
	}
}

// CloseWithTimeout closes a resource with a timeout
func CloseWithTimeout(r io.Closer, timeout time.Duration) error {
	if r == nil {
		return nil
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	
	ch := make(chan error, 1)
	
	go func() {
		ch <- r.Close()
	}()
	
	select {
	case err := <-ch:
		return err
	case <-ctx.Done():
		return fmt.Errorf("resource close timeout: %w", ctx.Err())
	}
}

// HttpServerResource is a resource wrapper for http.Server
type HttpServerResource struct {
	Server *http.Server // Capital S to export the field
}

// Close gracefully shuts down the HTTP server
func (r *HttpServerResource) Close() error {
	if r.Server == nil {
		return nil
	}
	
	// Create context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	log.Printf("Shutting down HTTP server on %s", r.Server.Addr)
	return r.Server.Shutdown(ctx)
}

// String returns a descriptive string for this resource
func (r *HttpServerResource) String() string {
	if r.Server == nil {
		return "HttpServer(nil)"
	}
	return fmt.Sprintf("HttpServer(%s)", r.Server.Addr)
}