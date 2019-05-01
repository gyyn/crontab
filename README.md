# crontab

etcd  
tar -zxvf etcd-v3.3.8-linux-amd64.tar.gz  
单机  
nohup ./etcd --listen-client-urls 'http://0.0.0.0:2379' --advertise-client-urls 'http://0.0.0.0:2379' &  
ETCDCTL_API=3 ./etcdctl put "name" "gyyn"  

集群  
nohup ./etcd --name centos1 \  
--initial-advertise-peer-urls http://192.168.0.111:2380 \  
--listen-peer-urls http://192.168.0.111:2380 \  
--listen-client-urls http://192.168.0.111:2379,http://127.0.0.1:2379 \  
--advertise-client-urls http://192.168.0.111:2379 \  
--initial-cluster-token etcd-cluster-1 \  
--initial-cluster centos1=http://192.168.0.111:2380,centos2=http://192.168.0.112:2380,centos3=http://192.168.0.113:2380 \  
--initial-cluster-state new &  

nohup ./etcd --name centos2 \  
--initial-advertise-peer-urls http://192.168.0.112:2380 \  
--listen-peer-urls http://192.168.0.112:2380 \  
--listen-client-urls http://192.168.0.112:2379,http://127.0.0.1:2379 \  
--advertise-client-urls http://192.168.0.112:2379 \  
--initial-cluster-token etcd-cluster-1 \  
--initial-cluster centos1=http://192.168.0.111:2380,centos2=http://192.168.0.112:2380,centos3=http://192.168.0.113:2380 \  
--initial-cluster-state new &  

nohup ./etcd --name centos3 \  
--initial-advertise-peer-urls http://192.168.0.113:2380 \  
--listen-peer-urls http://192.168.0.113:2380 \  
--listen-client-urls http://192.168.0.113:2379,http://127.0.0.1:2379 \  
--advertise-client-urls http://192.168.0.113:2379 \  
--initial-cluster-token etcd-cluster-1 \  
--initial-cluster centos1=http://192.168.0.111:2380,centos2=http://192.168.0.112:2380,centos3=http://192.168.0.113:2380 \  
--initial-cluster-state new &  

etcdctl cluster-health  
etcdctl member list  

交叉编译  
GOOS=linux GOARCH=amd64 go bulid src/github.com/gyyn/crontab/master/main/master.go  
GOOS=linux GOARCH=amd64 go bulid src/github.com/gyyn/crontab/worker/main/worker.go  