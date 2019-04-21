package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/elazarl/goproxy"
	"github.com/pkg/errors"

	"github.com/buzzers/rsyars/pkg/logger"
	"github.com/buzzers/rsyars/rsyars.adapter/hycdes"
	"github.com/buzzers/rsyars/rsyars.x/soc"
)

var (
	ch = make(chan []byte, 128)
)

func init() {

}

func main() {
	go loop()

	w, err := logger.NewWriter(fmt.Sprintf("rsyars.%d.log", time.Now().Unix()))
	if err != nil {
		log.Fatalf("创建日志记录器失败 -> %+v\n", err)
	}
	log.SetOutput(w)

	local, err := getLocalhost()
	if err != nil {
		log.Fatalf("获取代理地址失败 -> %+v\n", err)
	}

	log.Printf("代理地址 -> %s:8080\n", local)

	srv := goproxy.NewProxyHttpServer()
	srv.OnResponse(condition()).DoFunc(onResponse)
	srv.Logger = new(logger.NilLogger)

	if err := http.ListenAndServe(":8080", srv); err != nil {
		log.Fatalf("启动代理服务器失败 -> %+v\n", err)
	}
}

func build(body []byte) {
	type Girls struct {
		SoC map[string]*soc.SoC `json:"chip_with_user_info"`
	}

	_ = ioutil.WriteFile(fmt.Sprintf("response.%d.json", time.Now().Unix()), body, 0)

	girls := Girls{}
	if err := json.Unmarshal(body, &girls); err != nil {
		log.Printf("解析JSON数据失败 -> %+v\n", err)
		return
	}

	var values []*soc.SoC
	for _, c := range girls.SoC {
		values = append(values, c)
	}

	var targets []*hycdes.SoC
	for _, value := range values {
		target, err := hycdes.NewSoC(value)
		if err != nil {
			if !strings.Contains(err.Error(), "unknown") {
				log.Printf("解析芯片数据失败 -> %+v\n", err)
				return
			} else {
				continue
			}
		}
		targets = append(targets, target)
	}

	if len(targets) == 0 {
		return
	}

	c, err := hycdes.Build(targets)
	if err != nil {
		log.Printf("生成芯片代码失败 -> %+v\n", err)
		return
	}

	log.Printf("芯片代码 -> %s\n", c)
}

func loop() {
	for body := range ch {
		c := time.Now().Unix()
		log.Printf("处理响应数据 -> %d\n", c)
		if body == nil {
			log.Printf("响应数据为空 程序退出 -> %d\n", c)
			break
		}
		go build(body)
	}
}

func onResponse(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	log.Printf("处理请求响应 -> %s%s\n", ctx.Req.Host, ctx.Req.URL.Path)

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("读取响应数据失败 -> %+v\n", err)
		return resp
	}
	resp.Body = ioutil.NopCloser(bytes.NewBuffer(body))

	ch <- body

	return resp
}

func condition() goproxy.ReqConditionFunc {
	return func(req *http.Request, ctx *goproxy.ProxyCtx) bool {
		log.Printf("请求 -> %s%s\n", req.Host, req.URL.Path)
		if strings.HasSuffix(req.Host, "ppgame.com") {
			if strings.HasSuffix(req.URL.Path, "/Index/index") {
				log.Printf("请求通过 -> %s%s\n", req.Host, req.URL.Path)
				return true
			}
		}
		return false
	}
}

func getLocalhost() (string, error) {
	conn, err := net.Dial("tcp", "www.baidu.com:80")
	if err != nil {
		return "", errors.WithMessage(err, "连接 www.baidu.com:80 失败")
	}
	host, _, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return "", errors.WithMessage(err, "解析本地主机地址失败")
	}
	return host, nil
}
