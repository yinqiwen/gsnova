本项目衍生继承自[Snova](http://code.google.com/p/snova/)， 侧重于基于各种PaaS平台打造自用Proxy，兼顾一些反防火墙干扰的工程实现.

部署服务端
--------

[![Join the chat at https://gitter.im/gsnova/Lobby](https://badges.gitter.im/gsnova/Lobby.svg)](https://gitter.im/gsnova/Lobby?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge)
>目前支持Appengine（Google），Cloudfoudry/Openshift/Heroku/Dotcloud/Jelastic等PaaS平台。任选一个部署即可。
* Google AppEngine  
  用户需要先在[Google Appengine](http://appengine.google.com/)上注册帐号并创建appid。下载snova-gae-gserver-[version].zip，解压。windows用户直接执行deployer.exe，Linux/Mac用户需在命令行下执行python main.py，按部署工具提示进行部署。   
* Cloudfoudry/Openshift/Heroku/Dotcloud/Jelastic   
  下载snova-c4-server-[version].war。按照[部署说明](http://code.google.com/p/snova/w/list)部署。注意各个PaaS平台部署方式均不一致。  

>[下载](http://code.google.com/p/snova/downloads/list)


安装客户端
--------
>客户端为zip包，解压即可；目前预编译支持的有Windows（32/64位）， Linux（64位），Mac（64位）。   
用户按照[配置]一节修改配置后，即可启动gsnova。 windows用户直接执行gsnova.exe即可，Linux/Mac用户需要在命令行下启动gsnova程序。   
用户还需要修改浏览器的代理地址为127.0.0.1:48100， 或者在支持PAC设置的浏览器中设置PAC地址为http://127.0.0.1:48100/pac/gfwlist       
[下载](http://code.google.com/p/snova/downloads/list)

配置
-------
主要需要修改gsnova.conf，以下针对各个PaaS平台部署后配置说明   
#####Google Appengine
修改gsnova.conf中[GAE]以下部分，，默认Enable为1，关闭需要修改Enable为0：   

    [GAE]   
    Enable=1   
    WorkerNode[0]=myappid1   
将申请的appid填入WorkerNode[0]=后，注意appid为不含.appspot.com的前缀部分。若有多个appid，可以如下配置多个：

    [GAE]   
    Enable=1   
    WorkerNode[0]=myappid1 
    WorkerNode[1]=myappid2

注意暂时https的站点通过GAE代理是伪造证书方式实现的，若需要屏蔽浏览器警告，需要将conf目录下的Fake-ACRoot-Certificate.cer导入到浏览器中。
此Proxy实现在SPAC中名称为GAE

#####Cloudfoudry/Openshift/Heroku/Dotcloud/Appfog/Modulus 
修改gsnova.conf中[C4]以下部分，默认Enable为0，开启需要修改Enable为1：   

    [C4]   
    Enable=1   
    WorkerNode[0]=myapp.cloudfoundry.com   
将申请的域名填入WorkerNode[0]=后，注意必须为域名，且在配置前请确保在浏览器中输入此域名能看到‘snova’相关信息（证明部署服务端成功且能直接访问）。若有多个，可以如下配置多个：

    [C4]   
    Enable=1   
    WorkerNode[0]=myapp1.cloudfoundry.com
    WorkerNode[1]=myapp2.cloudfoundry.com

此Proxy实现在SPAC中名称为C4

#####SSH 
修改gsnova.conf中[C4]以下部分，默认Enable为0，开启需要修改Enable为1：   
SSH支持用户名密码校验方式，如下：

    [SSH]   
    Enable=1   
    WorkerNode[0]=ssh://user:passwd@sshhost:port

也支持RSA/DSA证书校验方式，这样提供SSH登录的Openshift/Dotcloud的ssh帐号即可用于此；配置如下：   

    [SSH]   
    Enable=1   
    WorkerNode[0]=ssh://user@host:port/?i=C:\Users\myname\.ssh\id_rsa
注意证书路径需要配置在i=后，为绝对路径   
此Proxy实现在SPAC中名称为SSH

SPAC(Special Auto Proxy Config)
-------
SPAC为gsnova中提供的一种灵活搭配各种proxy实现的胶水实现。借助于SPAC，可以结合各种Proxy实现的优点打造一个更好的组合Proxy。   
SPAC目前支持GAE, C4, SSH, Direct(直连)， Forward(中转到第三方Proxy)， GoogleHttps（用于google站点的反干扰）。Default为配置在[SPAC]下的Default， 默认Proxy实现。  
SPAC主要是一组规则，规则文件以JSON格式定义，例如：
     
       {
        "Host" : ["*.doubleclick.net*", "*.google-analytics.com*", "www.blogger.com", "*.google.co.*"],
        "Proxy":["GoogleHttps", "Direct", "Default"]
       }
       含义为若访问的目的站点域名匹配Hots中任意一个正则表达式，则选择Proxy中的三个Proxy。gsnova在三个Proxy优先选择第一个Proxy，若失败，则尝试第二个；若仍失败，则尝试第三个。  
       更多的参考spac目录下的cloud_spac.json，目前内置了一些规则实现。


自行编译
-------
> 有自行修改需求的用户请看这里：
>* 首先，需要安装Go编译器，[安装指南](http://golang.org/doc/install)；   
    主要设置两个环境变量：  
    
    export GOROOT=$HOME/go   //注意设置为Go编译器安装地址
    export PATH=$PATH:$GOROOT/bin
>* 下载gnsova源码，可用git下载

    git clone https://github.com/yinqiwen/gsnova.git

>* 编译   

    export GOPATH=GSnova Source Dir   //注意目录为绝对路径
    go get -u github.com/yinqiwen/godns   // 下载更新依赖的godns   
    go install -v ...   
可执行文件编译到了$GOPATH/bin下。注意此可执行文件依赖conf，web等目录下文件，不能直接运行。   
同时，提供了shell脚本简化编译打包过程。windows用户可在cygwin下执行./build.sh dist gsnova得到编译后的gsnova发布包。


其它
-------
参考[Snova](http://code.google.com/p/snova/)有一些其他相关信息