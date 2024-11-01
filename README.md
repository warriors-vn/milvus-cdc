# Milvus CDC Library [![GoDoc][doc-img]][doc] [![GoReport][go-report-img]][go-report]

``milvus-cdc`` A library for Change Data Capture (CDC) with Milvus, allowing you to capture, monitor, and respond to
changes in your Milvus database. Now, the library support milvus 1.x versions

<div align="center">
  <img src="https://media.giphy.com/media/ydPWRTNMR3x8mAWTdk/giphy.gif">
</div>

Installation
------------
    go get github.com/warriors-vn/milvus-cdc

Usage
-----

You can import ``milvus-cdc`` using a basic statement:

```go
import "github.com/warriors-vn/milvus-cdc"
```

Features
--------

- **Real-time change capture:** Detect insertions, updates, and deletions in Milvus collections.
- **Flexible event handling:** Configure custom event handlers for different change events.
- **High availability and scalability:** Compatible with distributed Milvus setups.
- **Easy integration:** Works with major programming languages.

Requirements
-----------

- **Milvus Server**: Version 1.x version
- **Docker** (if using Docker for deployment)
- **Programming Language Support**: Library supports Go, Python, etc. (based on your codebase)

Usage
-----

Configuration: Create a configuration file to specify the Milvus server connection details, event types,
and other CDC options.

Example:

```yaml
version: '3.8'

services:
  milvus-one:
    image: milvusdb/milvus:1.1.1-cpu-d061621-330cc6
    container_name: milvus-one
    ports:
      - "19530:19530"  # Milvus API port
      - "19121:19121"  # Monitoring port
    environment:
      DEPLOY_MODE: standalone  # Configure Milvus to run in standalone mode
    volumes:
      - milvus-data-one:/var/lib/milvus
      - ./server_config.yaml:/var/lib/milvus/conf/server_config.yaml

  milvus-two:
    image: milvusdb/milvus:1.1.1-cpu-d061621-330cc6
    container_name: milvus-two
    ports:
      - "29530:19530"  # Milvus API port
      - "29121:19121"  # Monitoring port
    environment:
      DEPLOY_MODE: standalone  # Configure Milvus to run in standalone mode
    volumes:
      - milvus-data-two:/var/lib/milvus
      - ./server_config.yaml:/var/lib/milvus/conf/server_config.yaml

  milvus-three:
    image: milvusdb/milvus:1.1.1-cpu-d061621-330cc6
    container_name: milvus-three
    ports:
      - "39530:19530"  # Milvus API port
      - "39121:19121"  # Monitoring port
    environment:
      DEPLOY_MODE: standalone  # Configure Milvus to run in standalone mode
    volumes:
      - milvus-data-three:/var/lib/milvus
      - ./server_config.yaml:/var/lib/milvus/conf/server_config.yaml

  milvus-em:
    image: milvusdb/milvus-em:latest
    container_name: milvus-em
    ports:
      - "3001:80"  # Milvus API port

volumes:
  milvus-data-one:
  milvus-data-two:
  milvus-data-three:
```

Example
-------

```go
package main

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/milvus-io/milvus-sdk-go/milvus"
	cdc "github.com/warriors-vn/milvus-cdc"
)

func main() {
	pubSub()
	queue()
}

func pubSub() {
	var sig chan os.Signal
	opts, err := redis.ParseURL("redis://default:@localhost:6379")
	if err != nil {
		return
	}

	opts.PoolSize = 30
	opts.DialTimeout = 10 * time.Second
	opts.ReadTimeout = 5 * time.Second
	opts.WriteTimeout = 5 * time.Second
	opts.IdleTimeout = 5 * time.Second
	opts.Username = ""

	redisCli := redis.NewClient(opts)

	err = redisCli.Ping(context.Background()).Err()
	if err != nil {
		return
	}

	milvusCli := make([]*cdc.MilvusClient, 0)
	ports := []string{"19530", "29530", "39530"}
	for _, port := range ports {
		conn, err := cdc.NewMilvusClient("0.0.0.0", port, cdc.DefaultTimeout)
		if err != nil {
			return
		}

		milvusCli = append(milvusCli, conn)
	}

	redisBroker := cdc.NewRedisBroker(redisCli, milvusCli)
	broker := cdc.NewBrokerFactory(redisBroker)
	worker := cdc.NewWorkerCDC(broker)

	go func() {
		defer func(worker *cdc.WorkerCDC, broker string) {
			errStop := worker.Stop(broker)
			if errStop != nil {
				return
			}
		}(worker, cdc.Redis)

		errStart := worker.Start(cdc.Redis, "test", cdc.PubSub)
		if errStart != nil {
			return
		}
	}()

	time.Sleep(5 * time.Second)

	// create collection
	msg := &cdc.MessageCDC{
		Action:         cdc.CreateCollection,
		CollectionName: "test_sync",
		Dimension:      512,
		IndexFileSize:  1024,
		MetricType:     milvus.IP,
	}

	marshal, err := json.Marshal(msg)
	if err != nil {
		return
	}

	_, err = cdc.NewRedisClient(redisCli).Publish(context.Background(), "test", string(marshal))
	if err != nil {
		return
	}

	// create index
	msg = &cdc.MessageCDC{
		Action:         cdc.CreateIndex,
		CollectionName: "test_sync",
		IndexType:      milvus.IVFFLAT,
		NList:          2048,
	}

	marshal, err = json.Marshal(msg)
	if err != nil {
		return
	}

	_, err = cdc.NewRedisClient(redisCli).Publish(context.Background(), "test", string(marshal))
	if err != nil {
		return
	}

	// insert vector
	msg = &cdc.MessageCDC{
		Action:         cdc.Insert,
		Vector:         "d1e4c2bc1e334c3d03bb09bb5043663d3782863d5390ec3cdf849a3bb15f0fbceda729b8af9326bc52a8ed3a674060bc89ffc73d1e8ebdbb79dc643d1bd81abd24a3a13c2907823d872989bd1c382cbd084fc23ca91f4abc866a39bcd07cc23c6036353dceb3c73c96e3903d77c6813c285aaebc723857bde38e5b3d2795943dc43486bc884c3cbb9ea9be3c4544afbdd6d3d73b4e5011bc5ebcacbc231bfc3c17ba94bd98019a3b3127a33ce8dddcbcb7ba5a3d4f30d2bc3f5657bc64ccfdbc62598bbcb095203badd22bbd6e4a243df9d7e5bbaa98183cff7f333cf35f2e3cf10dd0bc0b288a3c970724bdc138afbc704f273c84572fbd2195613d834fc73c567e11bd7eeaaf3d24116fbda7ee9d3c7d83193ce97ff03c129d4bbc4a171c3d2fac4cbdce4fee3bb89d293d8a86873d37270a3dfb82b03d9791d23d1436c5bd408c223caef13f3838c70ebd58fb933bdbc5563b82184dbc9c3ad9baa7c976bdeab62f3d1c599cbc1effc4bc65c5c13cdbe54abcb798e33cbc26d53c5a76713ab3aec9bc00ffc13d94ab873ccc5299bd4282623c393da43cc6dec6b99f37da3cc34d7bbc3c79793c7469e9bc1351d9bb4843f03cf900bfbb33391dbdfd7e16bdbae90e3df592c83d9bcee9bb2fc1b33cb03e223dd29d66bcc0d4e5bcfc46393c4d83823cd09baebc9129693d5d2fbab9b7fdf7bc54d4a4bc45f0c9bc46db7e3d0c90513cd0d72637b9d635bd7afe19bdf13142bd4ade3ebc52f41a3d165c013dc18cfebb8f2419bdfb952abd2fdef73cec1800bda451313d838231bc46ce0c3c41fb64bd1bf6403d36cd92bbe7be9c3d30ffa7bd65c8bdbd5073a83cfd37bf3de56823bde95883bcdf8517bd7de551bb1f8645bd6ceb093cb4202bbd7e9d9f3ca427263d6d2b89bbe8c40d3d43817c3cfa9dc43dace42cbdd59eefbbe95050bd9511833d6b80833cc0e5b63d62910abd4bdd623d40bbbcbb09019abc8c63e8bcecdd1abc0e8e30bd5b44333c25d679bc9c23aa3dd2c1303bb78f4c3cab5b13bdc8a3113d3e1252bd492b3ebca37347bc64ea0a3db447c53c0cea42bc768a07bca69f4e3da43ca13dfd35b23b327776bccb50773dc483aabbba66f73cea42413d38a74ebb5a39e63d9efbfb3bc92d8f3dfd23023d7dc5aebd72bc053c767f94bc526d813d0e88073d65b308bd3c781fbd6982cdbb42e6923b1a15383d1e9b4dbd573094bd18da9e3df8475a3de09a5abd9fc4ad3d7aa3583df2eae03b8991203d251339bd0dfd413d45621ebdb61e12bca576d4bc94e1f4bc640faa3d9a43dd3c3f05c7bcaedfb5bdc0dad23cadbe193da378fd3c2d2ad0bc1313e03ccbdb963b2998babdf18a86bd4ccf1fbd8c3744bda18f3d3d71ce963d6f4aed3c99c541bd0acd1b3d624b8b3d25120fbc2df448bd4346893d6b3a433caa71b0bcd6232e3d47cf9dbd724785bc8911a7bc0149163dfcfa00bb752aaf3b583b033b86a36c3da13c783c3c2ce8bc03564f3dc3e9afbc9a8093bcc59fd9bc814485bd375a8f3df648b6bcb145833b7b9ac03a0fbacf3c7e65d6bc72afd3bb1e36b7bc44b1dabd9723f33b24f374bd555f413dc7c12fbd2414b7bd6358633c423f33bc62bed8bc66e09bbaf6e9c03c107b8a3daaad61bd314043bc2f993cbd330badbd17aa6d3c364c773c503df23ce4b5483d1dbf8d3ccb70f33c5cbca53c61aae0bc0ed6af3c02c3a2bda9db283d18cfe53dc1060f3d442a9e3be27eeabcbb0ccd3d5ca2343cd9a8eebd9bd04f3c7f1f5e3ddc7ae33ca87981bc2f8ecd3c1fd7ad3b82c58fbdcd1ab23c71eb93bdd8da143db9c0463d12524ebcf5fbf7bc45df54bd71cee9bc53b2013de0d346bc04f2ae3d07d2e8bb8701723d8ecaae3cf817713d14fc4fbd509c98bd3973ba3a28472d3c37b998bdc23df53c6a645a3df60c12bb44ce4dbd3691a0bd0d3236bcf5d3efbc8aaadd3d6d4297bc9cd05d3d477095bd53bfc93d9430cabc70596f3dc94d543c3d9b24bd24a720bdfb1908bd2b251f3c2dbc1d3c77f56bbc723b283db359c6baa270073e4de3f6bc4c6a20bda6f544bd4daba7bdda1591bd9eef2b3cb5406b3dfcf2153c82dc99bcbd2383bb3fe7273de85c193e72a7673c8c6792bbbabeed3c2667b8bcf507a93bab9dc7bc8a7a0b3df710db3ce2b8f33c7a4b07bdeadba03d711c5a3d5d070abdb150c6bcbf81143db0ba1cbdcfb2883cb582033da667b6bcfd27d23b8446babcb63eb93c24f5afbdbfd8a2bd61b2d83dfc582cbd99052dbde18d6ebdb38a603d4766223df15c183ce941bebd6d02c83c4976063b140dbc3c985193bd3752963d5bde283d799070bdc976bebc1190383d09cf0dbc6654363d1ecb1a3cd427483d6381f63c382989bd44f245bcb1ece4bcab291bbdedc4253ceba7e6bbb0717a3da91cefbc4def4e3da3bd3cbc18200f3da2a5853dbaf048bd72f04b3d8aee5fbd522113ba7b1e81bdc067aabb4436afbc50bfce3afc7310bda7d8cb3bdd920e3d17be90bd3ea40ebdeccf4a3d09319a3dce3293bb794a32ba233b7abdb5237a3a830393bc0e043bbd4436a73d1adbdbbc1d4080bd5b5bca3c78d887bd99339ebd04156c3ce60c863d9d24993dd086163caf1e31bd7852c6bc3997a8bcd74223bd8e7724ba5ed099bdfd84bcbd829abb3bcbc00cbdce36ed3c571f293d250056bd1fcd34bcc0b2a13c8b8bddbc0d2a76bc9407953db27d6d3d2c82433df5ed293c78cb2d3c122e0e3ccbda923c379cde3c7c73a53c609c2ebaf0d76d3cae2a4cbd0a0a413ddf1471bba16758bc2dd2c43c7db150bcb4a0ed3c42c4923d3bbc45bb08e3e0bcd6c883bdf0b2403d0c1f473cf0714ebc0a26013df41d2abc27c603bc0c034d3d12077b3cb1e74f3c",
		CollectionName: "test_sync",
		PartitionTag:   "",
		Id:             1,
	}

	marshal, err = json.Marshal(msg)
	if err != nil {
		return
	}

	_, err = cdc.NewRedisClient(redisCli).Publish(context.Background(), "test", string(marshal))
	if err != nil {
		return
	}

	<-sig
}

func queue() {
	var sig chan os.Signal
	opts, err := redis.ParseURL("redis://default:@localhost:6379")
	if err != nil {
		return
	}

	opts.PoolSize = 30
	opts.DialTimeout = 10 * time.Second
	opts.ReadTimeout = 5 * time.Second
	opts.WriteTimeout = 5 * time.Second
	opts.IdleTimeout = 5 * time.Second
	opts.Username = ""

	redisCli := redis.NewClient(opts)

	err = redisCli.Ping(context.Background()).Err()
	if err != nil {
		return
	}

	milvusCli := make([]*cdc.MilvusClient, 0)
	ports := []string{"19530", "29530", "39530"}
	for _, port := range ports {
		conn, err := cdc.NewMilvusClient("0.0.0.0", port, cdc.DefaultTimeout)
		if err != nil {
			return
		}

		milvusCli = append(milvusCli, conn)
	}

	redisBroker := cdc.NewRedisBroker(redisCli, milvusCli)
	broker := cdc.NewBrokerFactory(redisBroker)
	worker := cdc.NewWorkerCDC(broker)

	go func() {
		defer func(worker *cdc.WorkerCDC, broker string) {
			errStop := worker.Stop(broker)
			if errStop != nil {
				return
			}
		}(worker, cdc.Redis)

		errStart := worker.Start(cdc.Redis, "test", cdc.Queue)
		if errStart != nil {
			return
		}
	}()

	time.Sleep(5 * time.Second)

	// insert vector
	msg := &cdc.MessageCDC{
		Action:         cdc.Insert,
		Vector:         "d1e4c2bc1e334c3d03bb09bb5043663d3782863d5390ec3cdf849a3bb15f0fbceda729b8af9326bc52a8ed3a674060bc89ffc73d1e8ebdbb79dc643d1bd81abd24a3a13c2907823d872989bd1c382cbd084fc23ca91f4abc866a39bcd07cc23c6036353dceb3c73c96e3903d77c6813c285aaebc723857bde38e5b3d2795943dc43486bc884c3cbb9ea9be3c4544afbdd6d3d73b4e5011bc5ebcacbc231bfc3c17ba94bd98019a3b3127a33ce8dddcbcb7ba5a3d4f30d2bc3f5657bc64ccfdbc62598bbcb095203badd22bbd6e4a243df9d7e5bbaa98183cff7f333cf35f2e3cf10dd0bc0b288a3c970724bdc138afbc704f273c84572fbd2195613d834fc73c567e11bd7eeaaf3d24116fbda7ee9d3c7d83193ce97ff03c129d4bbc4a171c3d2fac4cbdce4fee3bb89d293d8a86873d37270a3dfb82b03d9791d23d1436c5bd408c223caef13f3838c70ebd58fb933bdbc5563b82184dbc9c3ad9baa7c976bdeab62f3d1c599cbc1effc4bc65c5c13cdbe54abcb798e33cbc26d53c5a76713ab3aec9bc00ffc13d94ab873ccc5299bd4282623c393da43cc6dec6b99f37da3cc34d7bbc3c79793c7469e9bc1351d9bb4843f03cf900bfbb33391dbdfd7e16bdbae90e3df592c83d9bcee9bb2fc1b33cb03e223dd29d66bcc0d4e5bcfc46393c4d83823cd09baebc9129693d5d2fbab9b7fdf7bc54d4a4bc45f0c9bc46db7e3d0c90513cd0d72637b9d635bd7afe19bdf13142bd4ade3ebc52f41a3d165c013dc18cfebb8f2419bdfb952abd2fdef73cec1800bda451313d838231bc46ce0c3c41fb64bd1bf6403d36cd92bbe7be9c3d30ffa7bd65c8bdbd5073a83cfd37bf3de56823bde95883bcdf8517bd7de551bb1f8645bd6ceb093cb4202bbd7e9d9f3ca427263d6d2b89bbe8c40d3d43817c3cfa9dc43dace42cbdd59eefbbe95050bd9511833d6b80833cc0e5b63d62910abd4bdd623d40bbbcbb09019abc8c63e8bcecdd1abc0e8e30bd5b44333c25d679bc9c23aa3dd2c1303bb78f4c3cab5b13bdc8a3113d3e1252bd492b3ebca37347bc64ea0a3db447c53c0cea42bc768a07bca69f4e3da43ca13dfd35b23b327776bccb50773dc483aabbba66f73cea42413d38a74ebb5a39e63d9efbfb3bc92d8f3dfd23023d7dc5aebd72bc053c767f94bc526d813d0e88073d65b308bd3c781fbd6982cdbb42e6923b1a15383d1e9b4dbd573094bd18da9e3df8475a3de09a5abd9fc4ad3d7aa3583df2eae03b8991203d251339bd0dfd413d45621ebdb61e12bca576d4bc94e1f4bc640faa3d9a43dd3c3f05c7bcaedfb5bdc0dad23cadbe193da378fd3c2d2ad0bc1313e03ccbdb963b2998babdf18a86bd4ccf1fbd8c3744bda18f3d3d71ce963d6f4aed3c99c541bd0acd1b3d624b8b3d25120fbc2df448bd4346893d6b3a433caa71b0bcd6232e3d47cf9dbd724785bc8911a7bc0149163dfcfa00bb752aaf3b583b033b86a36c3da13c783c3c2ce8bc03564f3dc3e9afbc9a8093bcc59fd9bc814485bd375a8f3df648b6bcb145833b7b9ac03a0fbacf3c7e65d6bc72afd3bb1e36b7bc44b1dabd9723f33b24f374bd555f413dc7c12fbd2414b7bd6358633c423f33bc62bed8bc66e09bbaf6e9c03c107b8a3daaad61bd314043bc2f993cbd330badbd17aa6d3c364c773c503df23ce4b5483d1dbf8d3ccb70f33c5cbca53c61aae0bc0ed6af3c02c3a2bda9db283d18cfe53dc1060f3d442a9e3be27eeabcbb0ccd3d5ca2343cd9a8eebd9bd04f3c7f1f5e3ddc7ae33ca87981bc2f8ecd3c1fd7ad3b82c58fbdcd1ab23c71eb93bdd8da143db9c0463d12524ebcf5fbf7bc45df54bd71cee9bc53b2013de0d346bc04f2ae3d07d2e8bb8701723d8ecaae3cf817713d14fc4fbd509c98bd3973ba3a28472d3c37b998bdc23df53c6a645a3df60c12bb44ce4dbd3691a0bd0d3236bcf5d3efbc8aaadd3d6d4297bc9cd05d3d477095bd53bfc93d9430cabc70596f3dc94d543c3d9b24bd24a720bdfb1908bd2b251f3c2dbc1d3c77f56bbc723b283db359c6baa270073e4de3f6bc4c6a20bda6f544bd4daba7bdda1591bd9eef2b3cb5406b3dfcf2153c82dc99bcbd2383bb3fe7273de85c193e72a7673c8c6792bbbabeed3c2667b8bcf507a93bab9dc7bc8a7a0b3df710db3ce2b8f33c7a4b07bdeadba03d711c5a3d5d070abdb150c6bcbf81143db0ba1cbdcfb2883cb582033da667b6bcfd27d23b8446babcb63eb93c24f5afbdbfd8a2bd61b2d83dfc582cbd99052dbde18d6ebdb38a603d4766223df15c183ce941bebd6d02c83c4976063b140dbc3c985193bd3752963d5bde283d799070bdc976bebc1190383d09cf0dbc6654363d1ecb1a3cd427483d6381f63c382989bd44f245bcb1ece4bcab291bbdedc4253ceba7e6bbb0717a3da91cefbc4def4e3da3bd3cbc18200f3da2a5853dbaf048bd72f04b3d8aee5fbd522113ba7b1e81bdc067aabb4436afbc50bfce3afc7310bda7d8cb3bdd920e3d17be90bd3ea40ebdeccf4a3d09319a3dce3293bb794a32ba233b7abdb5237a3a830393bc0e043bbd4436a73d1adbdbbc1d4080bd5b5bca3c78d887bd99339ebd04156c3ce60c863d9d24993dd086163caf1e31bd7852c6bc3997a8bcd74223bd8e7724ba5ed099bdfd84bcbd829abb3bcbc00cbdce36ed3c571f293d250056bd1fcd34bcc0b2a13c8b8bddbc0d2a76bc9407953db27d6d3d2c82433df5ed293c78cb2d3c122e0e3ccbda923c379cde3c7c73a53c609c2ebaf0d76d3cae2a4cbd0a0a413ddf1471bba16758bc2dd2c43c7db150bcb4a0ed3c42c4923d3bbc45bb08e3e0bcd6c883bdf0b2403d0c1f473cf0714ebc0a26013df41d2abc27c603bc0c034d3d12077b3cb1e74f3c",
		CollectionName: "test_sync",
		PartitionTag:   "",
		Id:             1,
	}

	marshal, err := json.Marshal(msg)
	if err != nil {
		return
	}

	_, err = cdc.NewRedisClient(redisCli).LPush(context.Background(), "test", marshal)
	if err != nil {
		return
	}

	<-sig
}
```

Troubleshooting
---------------

- **Invalid CPU cache size error:** Adjust cache.cache_size in server_config.yaml to fit within system memory.
- **Connection issues:** Ensure Milvus server is running and accessible at the specified host and port.
- **Event lag:** Increase polling_interval in ``milvus-cdc-config.yaml`` to reduce system load.

Performance
-----------

Don't hesitate to participate in the discussion to enhance the generic helpers implementations.

Contributing
------------

* Ping me on instagram [@tuanelnino9](https://www.instagram.com/tuanelnino9)
* [Facebook](https://www.facebook.com/tuanelnino9)
* Linkedin [Tuan Nguyen Van](https://www.linkedin.com/in/tuan-nguyen-van-555315156)
* DMs, mentions, whatever, [send email](mailto:nguyenvantuan2391996@gmail.com) :))
* Fork the [project](https://github.com/warriors-vn/milvus-cdc)
* Fix [open issues](https://github.com/warriors-vn/milvus-cdc/issues) or request new features

Don't hesitate :))

Authors
-------

* Tuan Nguyen Van

[doc]: https://pkg.go.dev/github.com/warriors-vn/milvus-cdc
[doc-img]: https://pkg.go.dev/badge/github.com/warriors-vn/milvus-cdc
[go-report]: https://goreportcard.com/report/github.com/warriors-vn/milvus-cdc
[go-report-img]: https://goreportcard.com/badge/github.com/warriors-vn/milvus-cdc