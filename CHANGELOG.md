# Changelog

## [0.4.0](https://github.com/skaphos/fathom/compare/v0.3.1...v0.4.0) (2026-07-15)


### ⚠ BREAKING CHANGES

* **api:** the ClusterHealth CRD is now cluster-scoped. Upgrading requires deleting the old CRD (which removes existing ClusterHealth objects) and recreating aggregates as cluster-scoped resources.

### Features

* **adapters:** add KEDA, VPA, descheduler, and kured adapters ([#135](https://github.com/skaphos/fathom/issues/135)) ([8a5a6f3](https://github.com/skaphos/fathom/commit/8a5a6f34bd1b084f98d954d1d89ed0e8a1a68ff0))
* **adapter:** stabilize the contract at 1.0.0 (SKA-580) ([#139](https://github.com/skaphos/fathom/issues/139)) ([b2a38a8](https://github.com/skaphos/fathom/commit/b2a38a80dd7efe76d830eb84047bb01fcc324357))
* **api:** make ClusterHealth cluster-scoped (SKA-575) ([#137](https://github.com/skaphos/fathom/issues/137)) ([e35d574](https://github.com/skaphos/fathom/commit/e35d57468e9c53b07c311226899947ee20b71c36))


### Bug Fixes

* **api:** enforce checkRef and HealthReport spec immutability (SKA-576) ([#138](https://github.com/skaphos/fathom/issues/138)) ([6ff73a8](https://github.com/skaphos/fathom/commit/6ff73a8db00bcd5a0c5b18d020d0133214f80840))
* **deps:** upgrade Go toolchain to 1.26.5 and refresh dependencies ([#133](https://github.com/skaphos/fathom/issues/133)) ([4476d38](https://github.com/skaphos/fathom/commit/4476d386653938e10c2e2891a04095675a19fecf))
* harden addon policy and absence semantics ([#136](https://github.com/skaphos/fathom/issues/136)) ([9eba01f](https://github.com/skaphos/fathom/commit/9eba01f64e934244109a51af23ca317adaae7e37))

## [0.3.1](https://github.com/skaphos/fathom/compare/v0.3.0...v0.3.1) (2026-07-06)


### Bug Fixes

* clear stale aggregate status ([49aa696](https://github.com/skaphos/fathom/commit/49aa696fbe18e4f021eb219b46ce4b72d5dfa6b4))
* **controller:** reuse health reports after status conflicts ([c65ff5f](https://github.com/skaphos/fathom/commit/c65ff5fd40f24a210d22c4d9a5daf5d6346e607b))
* **controller:** validate health report reuse ([b74578b](https://github.com/skaphos/fathom/commit/b74578ba03e4cdea251706aff15942d9ee67beee))
* **controller:** validate node reports before adoption ([7af300e](https://github.com/skaphos/fathom/commit/7af300ed55aecd087e5c249d66f324d14555efd0))
* **healthcheck:** clarify namespace aggregation contract ([9703995](https://github.com/skaphos/fathom/commit/97039958788db3f30c5b5307fa2467ebb7fb84fe))
* **olm:** own node certificate checks ([11c2407](https://github.com/skaphos/fathom/commit/11c240777ef8e7492eb6a45d202f63960e7a2f0e))
* publish node-agent release image ([8ce80d6](https://github.com/skaphos/fathom/commit/8ce80d667d11de2342d9f83504a71d800a1c903b))
* require complete fresh node cert reports ([f150d07](https://github.com/skaphos/fathom/commit/f150d07ec15cb180877164212c3627be40b7575d))

## [0.0.2](https://github.com/skaphos/fathom/compare/v0.0.1...v0.0.2) (2026-05-17)


### Features

* **adapter:** add cert-manager resource health ([f70759a](https://github.com/skaphos/fathom/commit/f70759a99f5a820d922e9a7f7430ea9eb426f550))
* **adapter:** add cert-manager system health ([8eb7e75](https://github.com/skaphos/fathom/commit/8eb7e759e77a99dd62df3c80a78fa3f56b0e4014))
* **adapter:** add coredns health ([64fc678](https://github.com/skaphos/fathom/commit/64fc67841ccbee9a19a47fb9c34275eaf1fd407b))
* **adapter:** add external-secrets health ([fd6171c](https://github.com/skaphos/fathom/commit/fd6171ca68d0ffabbf1fb0c7a565784ae8c656de))
* **adapter:** add in-process adapter registry with capability discovery ([a61e1f8](https://github.com/skaphos/fathom/commit/a61e1f85d64112d307af975742da3f875eba16eb))
* **adapter:** complete cert-manager health probes ([5632e29](https://github.com/skaphos/fathom/commit/5632e298197da4f508c1237c56ce8689a417130d))
* **adapter:** coredns dns_resolution via probe pod (SKA-308) ([bbf00c2](https://github.com/skaphos/fathom/commit/bbf00c22044845197f9cbf60a90de649945f41d9))
* **adapter:** define AddonAdapter contract and version handshake ([#14](https://github.com/skaphos/fathom/issues/14)) ([4bafbd7](https://github.com/skaphos/fathom/commit/4bafbd74cb02cd6f0975cdf71496ec13c62ffa64))
* **adapter:** dry-run cert-manager admission ([953e094](https://github.com/skaphos/fathom/commit/953e094de79a6642248579b74a316252b66179b7))
* **adapter:** harden probe-pod namespace defaulting (SKA-313) ([73cbb73](https://github.com/skaphos/fathom/commit/73cbb73716a90b7e8c7370aebd53157041972b1e))
* **adapter:** operator-level --probe-image with contract bump (SKA-312) ([86b8ca0](https://github.com/skaphos/fathom/commit/86b8ca0affae8e2cc6569898364a752ef7d06e88))
* **adapter:** wire registry into manager ([c98c268](https://github.com/skaphos/fathom/commit/c98c268604a4dafc872684785fdadcd956266fe6))
* **addoncheck:** add API and controller scaffold ([d211d69](https://github.com/skaphos/fathom/commit/d211d69b14d9d81ad9f4b85608f1df65ec2af4a0))
* **addoncheck:** persist adapter health reports ([5b509db](https://github.com/skaphos/fathom/commit/5b509db52d1c675fa59b0958bbfd30e33ec5c7ca))
* **addoncheck:** retain bounded HealthReport history (SKA-288) ([3fc6658](https://github.com/skaphos/fathom/commit/3fc66589d174a8da07a95da0df5dbb5926cc89d4))
* **api:** give HealthCheck and ClusterHealth real v1alpha1 schemas (SKA-289) ([86e8132](https://github.com/skaphos/fathom/commit/86e8132cd684ca8694f21fb96ec0b63293480431))
* **ci:** publish multi-arch probe image to GHCR (SKA-311) ([#19](https://github.com/skaphos/fathom/issues/19)) ([ae2a2cb](https://github.com/skaphos/fathom/commit/ae2a2cb94d70b10ce7e32d618e9cc1f5fcfd8f6d))
* **controller:** implement ClusterHealthReconciler body (SKA-310) ([fb10d33](https://github.com/skaphos/fathom/commit/fb10d334cbe883b1d8a3d339447e7cbfda2c8ba0))
* **controller:** implement HealthCheckReconciler body (SKA-309) ([3ac0b21](https://github.com/skaphos/fathom/commit/3ac0b21c6f2dafb7a43b7c37a8194d9c6af40c98))
* **probe:** add lightweight probe pod foundation ([62e559f](https://github.com/skaphos/fathom/commit/62e559fe4af047e00193bca266e96b634d38d64d))
* **probe:** add probe pod launcher (SKA-307) ([732d973](https://github.com/skaphos/fathom/commit/732d9733578c3d1bd667005eada01a70979bd0c0))
* scaffold Fathom operator via operator-sdk ([8b4b10c](https://github.com/skaphos/fathom/commit/8b4b10c474562ba77e0ca2807d22e51a93996fb7))


### Bug Fixes

* **addoncheck:** surface missing adapter status ([02e9f12](https://github.com/skaphos/fathom/commit/02e9f12cb52f537c3cc435d824cab61d897d3ff9))
* **app:** gate /readyz on informer cache sync ([ab90781](https://github.com/skaphos/fathom/commit/ab90781f978466496ab8009186e16ea104e1b246))
* **controller:** preserve reconcile context loggers ([f81ad3c](https://github.com/skaphos/fathom/commit/f81ad3ce683a494cf8c21845c3bb71b428d39106))
* **controller:** use bound reconcile-stub logger (SKA-298) ([9c47a7a](https://github.com/skaphos/fathom/commit/9c47a7accc5d4962a5f931c26be87f02197c1f50))
