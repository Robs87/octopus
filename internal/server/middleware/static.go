package middleware

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func StaticEmbed(urlPrefix string, embedFS fs.FS) gin.HandlerFunc {
	fs := http.FS(embedFS)
	return static(urlPrefix, fs)
}

func StaticLocal(urlPrefix string, localPath string) gin.HandlerFunc {
	fs := http.Dir(localPath)
	return static(urlPrefix, fs)
}

func static(urlPrefix string, fileSystem http.FileSystem) gin.HandlerFunc {
	fileserver := http.FileServer(fileSystem)
	if urlPrefix != "" {
		fileserver = http.StripPrefix(urlPrefix, fileserver)
	}
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.Next()
			return
		}
		if _, err := fileSystem.Open(c.Request.URL.Path); err == nil {
			// 仅 /assets/ 目录下的内容哈希资源（JS/CSS/图片，文件名即版本）可长缓存 immutable；
			// 其余固定 URL 资源（HTML 入口、sw.js、manifest.json、favicon、logo 等）
			// 一律 no-cache——特别是 sw.js，若被 immutable 缓存会导致 Service Worker
			// 永不更新，旧 SW 及其旧缓存会持续向用户提供旧版页面（表现为"改了代码看不到"）。
			if strings.HasPrefix(c.Request.URL.Path, "/assets/") {
				c.Header("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				c.Header("Cache-Control", "no-cache")
			}
			fileserver.ServeHTTP(c.Writer, c.Request)
			c.Abort()
		}
	}
}
