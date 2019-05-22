package master

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"time"

	"github.com/pkg/sftp"

	"golang.org/x/crypto/ssh"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/gyyn/crontab/common"
)

///cron/workers/
type WorkerMgr struct {
	client *clientv3.Client
	kv     clientv3.KV
	lease  clientv3.Lease
}

var (
	G_workerMgr *WorkerMgr
)

//获取在线worker列表
func (workerMgr *WorkerMgr) ListWorkers() (workerArr []string, err error) {
	var (
		getResp  *clientv3.GetResponse
		kv       *mvccpb.KeyValue
		workerIP string
	)

	//初始化数组
	workerArr = make([]string, 0)

	//获取目录下所有Kv
	if getResp, err = workerMgr.kv.Get(context.TODO(), common.JOB_WORKER_DIR, clientv3.WithPrefix()); err != nil {
		return
	}

	//解析每个节点的IP
	for _, kv = range getResp.Kvs {
		// kv.Key : /cron/workers/192.168.1.2
		workerIP = common.ExtractWorkerIP(string(kv.Key))
		workerArr = append(workerArr, workerIP)
	}
	return
}

func InitWorkerMgr() (err error) {
	var (
		config clientv3.Config
		client *clientv3.Client
		kv     clientv3.KV
		lease  clientv3.Lease
	)

	//初始化配置
	config = clientv3.Config{
		Endpoints:   G_config.EtcdEndpoints,                                     //集群地址
		DialTimeout: time.Duration(G_config.EtcdDialTimeout) * time.Millisecond, //连接超时
	}

	//建立连接
	if client, err = clientv3.New(config); err != nil {
		return
	}

	//得到KV和Lease的API子集
	kv = clientv3.NewKV(client)
	lease = clientv3.NewLease(client)

	G_workerMgr = &WorkerMgr{
		client: client,
		kv:     kv,
		lease:  lease,
	}
	return
}

type SSHCli struct {
	user       string
	pwd        string
	addr       string
	sshClient  *ssh.Client
	sftpClient *sftp.Client
	session    *ssh.Session
	LastResult string
}

func (c *SSHCli) Connect() (*SSHCli, error) {
	config := &ssh.ClientConfig{}
	config.SetDefaults()
	config.User = c.user
	config.Auth = []ssh.AuthMethod{ssh.Password(c.pwd)}
	config.HostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error { return nil }
	sshClient, err := ssh.Dial("tcp", c.addr, config)
	if err != nil {
		return c, err
	}
	c.sshClient = sshClient

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return c, err
	}
	c.sftpClient = sftpClient

	fmt.Println("connect!")
	return c, nil
}

func (c SSHCli) Run(shell string) (string, error) {
	if c.sshClient == nil {
		if _, err := c.Connect(); err != nil {
			return "", err
		}
	}

	session, err := c.sshClient.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	buf, err := session.CombinedOutput(shell)

	c.LastResult = string(buf)
	return c.LastResult, err
}

func (c SSHCli) SendFile(localFilePath string, remoteDir string) error {
	if c.sshClient == nil {
		if _, err := c.Connect(); err != nil {
			return err
		}
	}

	srcFile, err := os.Open(localFilePath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	var remoteFileName = path.Base(localFilePath)
	dstFile, err := c.sftpClient.Create(path.Join(remoteDir, remoteFileName))
	if err != nil {
		return err
	}
	defer dstFile.Close()

	ff, err := ioutil.ReadAll(srcFile)
	if err != nil {
		return err
	}
	dstFile.Write(ff)

	return err
}
