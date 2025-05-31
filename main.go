package main

import (
	"io"
	"log"
	"net"
	"net/http"
)

// handleHTTP 处理普通 HTTP 请求
func handleHTTP(w http.ResponseWriter, req *http.Request) {
	// 创建 HTTP 客户端
	client := &http.Client{}

	// 复制请求，移除代理相关头
	targetReq, err := http.NewRequest(req.Method, req.URL.String(), req.Body)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		log.Printf("Error creating request: %v", err)
		return
	}

	// 复制请求头
	for key, values := range req.Header {
		for _, value := range values {
			targetReq.Header.Add(key, value)
		}
	}

	// 发送请求到目标服务器
	resp, err := client.Do(targetReq)
	if err != nil {
		http.Error(w, "Failed to forward request", http.StatusBadGateway)
		log.Printf("Error forwarding request: %v", err)
		return
	}
	defer resp.Body.Close()

	// 复制响应头
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// 设置状态码
	w.WriteHeader(resp.StatusCode)

	// 复制响应体
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("Error copying response: %v", err)
	}
}

// handleConnect 处理 HTTPS CONNECT 请求
func handleConnect(w http.ResponseWriter, req *http.Request) {
	// 连接目标服务器
	destConn, err := net.Dial("tcp", req.URL.Host)
	if err != nil {
		http.Error(w, "Failed to connect to target", http.StatusBadGateway)
		log.Printf("Error connecting to target %s: %v", req.URL.Host, err)
		return
	}
	defer destConn.Close()

	// 劫持客户端连接
	clientConn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		http.Error(w, "Failed to hijack connection", http.StatusInternalServerError)
		log.Printf("Error hijacking connection: %v", err)
		return
	}
	defer clientConn.Close()

	// 发送 200 Connection Established
	_, err = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		log.Printf("Error sending 200 response: %v", err)
		return
	}

	// 双向数据转发
	go func() {
		_, err := io.Copy(destConn, clientConn)
		if err != nil {
			log.Printf("Error copying from client to dest: %v", err)
		}
	}()
	_, err = io.Copy(clientConn, destConn)
	if err != nil {
		log.Printf("Error copying from dest to client: %v", err)
	}
}

// proxyHandler 处理所有代理请求
func proxyHandler(w http.ResponseWriter, req *http.Request) {
	log.Printf("Received %s request for %s", req.Method, req.URL.String())

	if req.Method == http.MethodConnect {
		handleConnect(w, req)
	} else {
		handleHTTP(w, req)
	}
}

func main() {
	// 设置代理服务器
	server := &http.Server{
		Addr:    ":8080",
		Handler: http.HandlerFunc(proxyHandler),
	}

	log.Printf("Starting proxy server on :8080")
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
