package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/elazarl/goproxy"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"

	log_std "github.com/buzzers/rsyars/pkg/log"
	"github.com/buzzers/rsyars/pkg/util"
	"github.com/buzzers/rsyars/rsyars.adapter/hycdes"
	"github.com/buzzers/rsyars/rsyars.x/soc"
)

func main() {
	log, err := log_std.New(fmt.Sprintf("rsyars.%d.log", time.Now().Unix()))
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}

	conf := confT{
		Verbose: false,
		Rule:    []string{"02"},
	}
	body, err := ioutil.ReadFile("rsyars.yaml")
	if err != nil {
		log.Errorf("读取配置文件失败并使用默认配置 -> %+v", err)
	} else {
		value := new(confT)
		if err := yaml.Unmarshal(body, value); err != nil {
			log.Errorf("解析配置文件失败并使用默认配置 -> %+v", err)
		}
		conf = *value
	}
	for _, rule := range conf.Rule {
		if rule[0] != '0' && rule[0] != '1' && rule[0] != '2' ||
			rule[1] != '0' && rule[1] != '1' && rule[1] != '2' {
			log.Fatalf("规则格式错误 -> %s", rule)
		}
	}

	rsyars := &rsyars{
		log:  log,
		ch:   make(chan response, 128),
		conf: conf,
	}
	if err := rsyars.Run(); err != nil {
		rsyars.log.Fatalf("程序启动失败 -> %+v", err)
	}
}

type confT struct {
	Verbose bool     `yaml:"verbose"`
	Rule    []string `yaml:"rule"`
}

type response struct {
	Host string
	Path string
	Body []byte
}

type rsyars struct {
	log  log_std.Logger
	ch   chan response
	conf confT
}

func (rs *rsyars) Run() error {
	go rs.loop()

	localhost, err := rs.getLocalhost()
	if err != nil {
		rs.log.Fatalf("获取代理地址失败 -> %+v", err)
	}

	rs.log.Tipsf("代理地址 -> %s:8080", localhost)

	srv := goproxy.NewProxyHttpServer()
	srv.Logger = new(util.NilLogger)
	srv.OnResponse(rs.condition()).DoFunc(rs.onResponse)

	if err := http.ListenAndServe(":8080", srv); err != nil {
		rs.log.Fatalf("启动代理服务器失败 -> %+v", err)
	}

	return nil
}

func (rs *rsyars) build(body response) {
	type Girls struct {
		SoC map[string]*soc.SoC `json:"chip_with_user_info"`
	}

	if rs.conf.Verbose {
		_ = ioutil.WriteFile(fmt.Sprintf("response.%d.json", time.Now().Unix()), body.Body, 0)
	}

	girls := Girls{}
	if err := json.Unmarshal(body.Body, &girls); err != nil {
		rs.log.Errorf("解析JSON数据失败 -> %+v", err)
		return
	}

	var values []*soc.SoC
	for _, c := range girls.SoC {
		values = append(values, c)
	}

	var targets []*hycdes.SoC
	for _, value := range values {
		if !rs.pass(value) {
			continue
		}
		target, err := hycdes.NewSoC(value)
		if err != nil {
			if !strings.Contains(err.Error(), "unknown") {
				rs.log.Errorf("解析芯片数据失败 -> %+v", err)
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
		rs.log.Errorf("生成芯片代码失败 -> %+v", err)
		return
	}

	rs.log.Tipsf("芯片代码 -> %s", c)
	if !clipboard.Unsupported {
		if err := clipboard.WriteAll(c); err != nil {
			rs.log.Errorf("复制芯片代码到剪贴板失败 -> %+v", err)
		} else {
			rs.log.Tipsf("芯片代码已复制到剪贴板")
		}
	}
}

func (rs *rsyars) loop() {
	for body := range rs.ch {
		c := time.Now().Unix()
		rs.log.Infof("处理响应数据 -> %d", c)
		if body.Body == nil {
			rs.log.Infof("响应数据为空 程序退出 -> %d", c)
			break
		}
		go rs.build(body)
	}
}

func (rs *rsyars) onResponse(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	rs.log.Infof("处理请求响应 -> %s", path(ctx.Req))

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		rs.log.Errorf("读取响应数据失败 -> %+v", err)
		return resp
	}
	resp.Body = ioutil.NopCloser(bytes.NewBuffer(body))

	rs.ch <- response{
		Host: ctx.Req.Host,
		Path: ctx.Req.URL.Path,
		Body: body,
	}

	return resp
}

func (rs *rsyars) condition() goproxy.ReqConditionFunc {
	return func(req *http.Request, ctx *goproxy.ProxyCtx) bool {
		rs.log.Infof("请求 -> %s", path(req))
		if strings.HasSuffix(req.Host, "ppgame.com") {
			if strings.HasSuffix(req.URL.Path, "/Index/index") {
				rs.log.Infof("请求通过 -> %s", path(req))
				return true
			}
		}
		return false
	}
}

func (rs *rsyars) getLocalhost() (string, error) {
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

func (rs *rsyars) pass(value *soc.SoC) bool {
	for _, rule := range rs.conf.Rule {
		switch rule[0] {
		case '0':
			switch rule[1] {
			case '0':
				return true
			case '1':
				return value.SquadWithUserID != "0"
			case '2':
				return value.SquadWithUserID == "0"
			}
		case '1':
			switch rule[1] {
			case '0':
				return value.Locked != "0"
			case '1':
				return value.Locked != "0" && value.SquadWithUserID != "0"
			case '2':
				return value.Locked != "0" && value.SquadWithUserID == "0"
			}
		case '2':
			switch rule[1] {
			case '0':
				return value.Locked == "0"
			case '1':
				return value.Locked == "0" && value.SquadWithUserID != "0"
			case '2':
				return value.Locked == "0" && value.SquadWithUserID == "0"
			}
		}
	}
	return false
}

func path(req *http.Request) string {
	if req.URL.Path == "/" {
		return req.Host
	}
	return req.Host + req.URL.Path
}
