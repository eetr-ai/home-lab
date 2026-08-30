# Changelog

## [1.5.3](https://github.com/eetr-ai/home-lab/compare/admin-v1.5.2...admin-v1.5.3) (2026-08-30)


### Bug Fixes

* **admin-web:** put the pipeline snippet behind a button and fold the log ([#76](https://github.com/eetr-ai/home-lab/issues/76)) ([e27cbe2](https://github.com/eetr-ai/home-lab/commit/e27cbe2596ae7b88089fad8a512f992035be9977))

## [1.5.2](https://github.com/eetr-ai/home-lab/compare/admin-v1.5.1...admin-v1.5.2) (2026-08-30)


### Bug Fixes

* **admin-web:** stop asking for a client id as a resource indicator ([#74](https://github.com/eetr-ai/home-lab/issues/74)) ([ad39e50](https://github.com/eetr-ai/home-lab/commit/ad39e508d55ce3bd565f3719e7c8bd7cc33e6b56))

## [1.5.1](https://github.com/eetr-ai/home-lab/compare/admin-v1.5.0...admin-v1.5.1) (2026-08-30)


### Bug Fixes

* **admin-api:** apply as "helm" so the panel is not a second writer ([#72](https://github.com/eetr-ai/home-lab/issues/72)) ([d7fc8f1](https://github.com/eetr-ai/home-lab/commit/d7fc8f17c6b49c8a35bf71ea7e5184cf64ea9142))

## [1.5.0](https://github.com/eetr-ai/home-lab/compare/admin-v1.4.0...admin-v1.5.0) (2026-08-30)


### Features

* deploy from a pipeline with an eetr-auth API key ([#70](https://github.com/eetr-ai/home-lab/issues/70)) ([d1cbb1b](https://github.com/eetr-ai/home-lab/commit/d1cbb1b45666b944ec7141f145ec3d78bdb40a73))

## [1.4.0](https://github.com/eetr-ai/home-lab/compare/admin-v1.3.0...admin-v1.4.0) (2026-08-30)


### Features

* **admin-api:** add the namespace protection policy ([#65](https://github.com/eetr-ai/home-lab/issues/65)) ([d337e8d](https://github.com/eetr-ai/home-lab/commit/d337e8dfe46f3824c6945c5b4c6ecfa31ce55262))
* **admin-api:** create and delete cluster namespaces ([#43](https://github.com/eetr-ai/home-lab/issues/43)) ([583aa40](https://github.com/eetr-ai/home-lab/commit/583aa4038548a3cac1540d22ee08483a51c4f9f5))
* **admin-api:** declare, version, and roll out Helm deployments ([#54](https://github.com/eetr-ai/home-lab/issues/54)) ([7459d54](https://github.com/eetr-ai/home-lab/commit/7459d549247f69e76a5869a7eb803cf6bbb0b03a))
* **admin-api:** install charts by reference, and take values as YAML ([#53](https://github.com/eetr-ai/home-lab/issues/53)) ([17d1aa4](https://github.com/eetr-ai/home-lab/commit/17d1aa4189b07d912232d5175d7fe7a56e95d9b2))
* **admin-api:** read Helm releases through the Helm SDK ([#45](https://github.com/eetr-ai/home-lab/issues/45)) ([2dbdd1c](https://github.com/eetr-ai/home-lab/commit/2dbdd1c8a359118691a3dd3f90399862a3d6e470))
* **admin-api:** resolve the caller on every request ([#64](https://github.com/eetr-ai/home-lab/issues/64)) ([5fe1a9b](https://github.com/eetr-ai/home-lab/commit/5fe1a9bfacd06266e0ea40c36eb0c51ed0b04eac))
* **admin-web:** edit Helm values and roll them out from the panel ([#55](https://github.com/eetr-ai/home-lab/issues/55)) ([81d6dd8](https://github.com/eetr-ai/home-lab/commit/81d6dd897b1653f982b1286b5a4e21fbd8678173))
* **admin-web:** give the Helm section a dashboard, and somewhere to go back to ([#60](https://github.com/eetr-ai/home-lab/issues/60)) ([470717b](https://github.com/eetr-ai/home-lab/commit/470717b20aae3baab0b9f81002d1ad7e36604e54))
* **admin-web:** manage namespaces from the cluster section ([#44](https://github.com/eetr-ai/home-lab/issues/44)) ([05abb03](https://github.com/eetr-ai/home-lab/commit/05abb031076116fda9285a8ed5adbd972c5f4412))
* **admin:** let the panel deploy its own namespace, and its own release ([#62](https://github.com/eetr-ai/home-lab/issues/62)) ([dd910f3](https://github.com/eetr-ai/home-lab/commit/dd910f33b66d0e918ef64e9f73af975b965cebde))
* **admin:** run Helm operations in a Job, not in the API's own pods ([#68](https://github.com/eetr-ai/home-lab/issues/68)) ([68327ee](https://github.com/eetr-ai/home-lab/commit/68327ee84a2394a3fe7ae5a66704ca942d6caa22))

## [1.3.0](https://github.com/eetr-ai/home-lab/compare/admin-v1.2.0...admin-v1.3.0) (2026-08-28)


### Features

* **admin-agent:** upgrade to octo v0.8.8, and take the memory it now offers ([#35](https://github.com/eetr-ai/home-lab/issues/35)) ([34f575a](https://github.com/eetr-ai/home-lab/commit/34f575afb31e67aa5e275f4f8950b811e7984d76))
* **admin-web:** coordinate the token refresh, and run two replicas ([#38](https://github.com/eetr-ai/home-lab/issues/38)) ([ccf0fe9](https://github.com/eetr-ai/home-lab/commit/ccf0fe96c3699d1ea003b9557c18fa212dbea4d5))
* **charts:** run the API at two, add Redis, and keep what storage is given ([#36](https://github.com/eetr-ai/home-lab/issues/36)) ([0afb4a1](https://github.com/eetr-ai/home-lab/commit/0afb4a11d9dacc19e6df7e0389e970deb36f0371))

## [1.2.0](https://github.com/eetr-ai/home-lab/compare/admin-v1.1.0...admin-v1.2.0) (2026-08-26)


### Features

* **admin-web:** the assistant's drawer, its Markdown, and page scope in the URL ([#32](https://github.com/eetr-ai/home-lab/issues/32)) ([909e6fc](https://github.com/eetr-ai/home-lab/commit/909e6fc0268a4408b24d1f595df686e140d58bf6))
* Sous, an assistant for the admin panel ([#29](https://github.com/eetr-ai/home-lab/issues/29)) ([ce7831c](https://github.com/eetr-ai/home-lab/commit/ce7831c5dd41dc8a627a0781a86816f3b265f933))


### Bug Fixes

* **admin-api:** the query console runs as the panel's own superuser ([#30](https://github.com/eetr-ai/home-lab/issues/30)) ([ad5768c](https://github.com/eetr-ai/home-lab/commit/ad5768c3a39c430d624bc2780f20ae9e150bc7a9))

## [1.1.0](https://github.com/eetr-ai/home-lab/compare/admin-v1.0.0...admin-v1.1.0) (2026-08-26)


### Features

* **admin:** dashboard with cluster metrics, nodes, and storage ([#24](https://github.com/eetr-ai/home-lab/issues/24)) ([a2498cd](https://github.com/eetr-ai/home-lab/commit/a2498cd7de2fabb801549d7582aebb4119774ec1))

## 1.0.0 (2026-08-26)


### Features

* **admin-api:** manage MongoDB databases, collections, and users ([#13](https://github.com/eetr-ai/home-lab/issues/13)) ([81d375c](https://github.com/eetr-ai/home-lab/commit/81d375cbcbc4437036ce5ad25350e569ac6f4eea))
* **admin-api:** manage PostgreSQL databases, roles, and extensions ([#12](https://github.com/eetr-ai/home-lab/issues/12)) ([3d1c2fa](https://github.com/eetr-ai/home-lab/commit/3d1c2fa7474f8860d672184703be79d0672a965b))
* **admin-api:** read the cluster ([#14](https://github.com/eetr-ai/home-lab/issues/14)) ([d640c9f](https://github.com/eetr-ai/home-lab/commit/d640c9ff8fdb3b9ebaf14f63c420a428de7a82a4))
* **admin-api:** serve health, auth, and the OpenAPI description ([#11](https://github.com/eetr-ai/home-lab/issues/11)) ([768cc5e](https://github.com/eetr-ai/home-lab/commit/768cc5ee3754b1f5432512a31d01905c1dcbde17))
* **admin-web:** manage the databases and read the cluster ([#16](https://github.com/eetr-ai/home-lab/issues/16)) ([fc85969](https://github.com/eetr-ai/home-lab/commit/fc85969989b13b34051ed3cd5cdf58e98baa3ceb))
* **admin-web:** sign in against eetr-auth and call the API as the operator ([#15](https://github.com/eetr-ai/home-lab/issues/15)) ([27c2ae1](https://github.com/eetr-ai/home-lab/commit/27c2ae11247c1270e555057527ccef47e9bd36dc))
