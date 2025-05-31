package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

// handleHTTP 处理普通 HTTP 请求
func handleHTTP(w http.ResponseWriter, req *http.Request) {
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
		Transport: &http.Transport{
			MaxIdleConns:        100,
			IdleConnTimeout:     30 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
			DisableCompression:  false,
		},
	}

	if req.URL.Scheme == "" {
		req.URL.Scheme = "http"
	}

	targetReq, err := http.NewRequest(req.Method, req.URL.String(), req.Body)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		log.Printf("Error creating request: %v", err)
		return
	}

	for key, values := range req.Header {
		for _, value := range values {
			targetReq.Header.Add(key, value)
		}
	}
	targetReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	targetReq.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	targetReq.Header.Set("Connection", "keep-alive")
	targetReq.Header.Set("Cookie", "zhwikiVariant=zh-cn")

	resp, err := client.Do(targetReq)
	if err != nil {
		http.Error(w, "Failed to forward request", http.StatusBadGateway)
		log.Printf("Error forwarding request to %s: %v", req.URL.String(), err)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("Error copying response: %v", err)
	}
}

// handleConnect 处理 HTTPS CONNECT 请求
func handleConnect(w http.ResponseWriter, req *http.Request) {
	// 增加超时和错误处理
	destConn, err := net.DialTimeout("tcp", req.URL.Host, 10*time.Second)
	if err != nil {
		http.Error(w, "Failed to connect to target", http.StatusBadGateway)
		log.Printf("Error connecting to target %s: %v", req.URL.Host, err)
		return
	}
	defer destConn.Close()

	clientConn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		http.Error(w, "Failed to hijack connection", http.StatusInternalServerError)
		log.Printf("Error hijacking connection: %v", err)
		return
	}
	defer clientConn.Close()

	// 设置写入超时
	if err := clientConn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		log.Printf("Error setting write deadline: %v", err)
		return
	}

	// 发送 200 响应
	_, err = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		log.Printf("Error sending 200 response: %v", err)
		return
	}

	// 双向数据转发
	done := make(chan struct{})
	go func() {
		_, err := io.Copy(destConn, clientConn)
		if err != nil {
			log.Printf("Error copying from client to dest: %v", err)
		}
		close(done)
	}()
	_, err = io.Copy(clientConn, destConn)
	if err != nil {
		log.Printf("Error copying from dest to client: %v", err)
	}
	<-done
}

// proxyHandler 处理所有代理请求
func proxyHandler(w http.ResponseWriter, req *http.Request) {
	log.Printf("Received %s request for %s from %s", req.Method, req.URL.String(), req.RemoteAddr)
	if req.Method == http.MethodConnect {
		handleConnect(w, req)
	} else {
		handleHTTP(w, req)
	}
}

func main() {
	server := &http.Server{
		Addr:         ":8080",
		Handler:      http.HandlerFunc(proxyHandler),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("Starting proxy server on :8080")
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
