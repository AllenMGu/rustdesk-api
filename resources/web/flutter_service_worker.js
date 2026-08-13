'use strict';
const MANIFEST = 'flutter-app-manifest';
const TEMP = 'flutter-temp-cache';
const CACHE_NAME = 'flutter-app-cache';
const RESOURCES = {
  "AGPL-3.0.txt": "4ae09d45eac4aa08d013b5f2e01c67f6",
  "NOTICE": "0523caff40e22493a524ffeb38d60058",
  "SOURCE.html": "74614c9eb6d39a38658eaea308c39ffc",
  "assets/AssetManifest.json": "8d7619e36fd23ae0a6e072735d05043f",
  "assets/FontManifest.json": "1053b6a7ac98b1998f83026f68cea87b",
  "assets/NOTICES": "5550015e65deb04cb66d6d6a7719a454",
  "assets/assets/android.png": "3856527a965dad9a4b4625ec5c83ed1b",
  "assets/assets/gestures.ttf": "a70c60208ba07ce378ed9a5cf8aa586b",
  "assets/assets/insecure.png": "83eb7acf1ccc3fc61da4a5b24c5c2097",
  "assets/assets/insecure_relay.png": "3c009d927b3a8d5ab209758a2b87d710",
  "assets/assets/linux.png": "b3282723058972022cfd61b5395cd52f",
  "assets/assets/mac.png": "6c71189768c9f28906fa157d227bd2ff",
  "assets/assets/secure.png": "2f77df392c6faa3d8a94dc42f916f99e",
  "assets/assets/secure_relay.png": "ff760a9494c07985683173f362d4631c",
  "assets/assets/win.png": "e1a0227db222645b8efc9891011ece1f",
  "assets/fonts/MaterialIcons-Regular.otf": "e7069dfd19b331be16bed984668fe080",
  "assets/packages/cupertino_icons/assets/CupertinoIcons.ttf": "6d342eb68f170c97609e9da345464e5e",
  "assets/packages/wakelock_web/assets/no_sleep.js": "7748a45cd593f33280669b29c2c8919a",
  "canvaskit/canvaskit.js": "97937cb4c2c2073c968525a3e08c86a3",
  "canvaskit/canvaskit.wasm": "3de12d898ec208a5f31362cc00f09b9e",
  "canvaskit/profiling/canvaskit.js": "c21852696bc1cc82e8894d851c01921a",
  "canvaskit/profiling/canvaskit.wasm": "371bc4e204443b0d5e774d64a046eb99",
  "favicon.svg": "8452bf248b5b5519dea5f3c724bbac9d",
  "flutter.js": "a85fcf6324d3c4d3ae3be1ae4931e9c5",
  "icons/Icon-192.png": "ecb50d026dad007de7c81e675bda5ab7",
  "icons/Icon-512.png": "363d0fbd6c49a37ee01b9afad0849d86",
  "icons/Icon-maskable-192.png": "5496a0a956a65451bb26d515a8365838",
  "icons/Icon-maskable-512.png": "7f03ce96eb8a827eb1a8dd06a5ebc638",
  "index.html": "04073c04d7d82076c996fce06ff6a5ef",
  "js/dist/index.css": "325598511be4c5926c2e5bd308a3ea4c",
  "js/dist/index.html": "85716a9a0538b47b808045f6923bc8d8",
  "js/dist/index.js": "0e9753b63e96ff63087ca3cbbb8af346",
  "js/dist/vendor.js": "158a0d2d986f84d64c5e9a2171201bb6",
  "js/gen_js_from_hbb.py": "b6e1ca5f641d115bef1cad62ea610cba",
  "js/index.html": "2e82f245c8a1b4a297893becc211e8ba",
  "js/package-lock.json": "fd6bde78f3b8327e17a9dccd21363182",
  "js/package.json": "397649f693f9e2f3052c1ce2c5fb925f",
  "js/src/codec.js": "9e5faf3ec7cf3a33f1bedfb798ea0aa7",
  "js/src/common.ts": "5b4db4beb519ea747aa575116dd09f32",
  "js/src/connection.ts": "5a895157e1260a106c6d4306236424e6",
  "js/src/gen_js_from_hbb.ts": "ffa7bd435d554799387002e6d8488529",
  "js/src/globals.js": "141947c6e83e29843ccb280ae5cd4616",
  "js/src/ljw.js": "5370a93c35fa65ff6cc3950756f8c437",
  "js/src/main.ts": "b0f2bfbc183c87e47c5ed802a54fbfa6",
  "js/src/message.ts": "6afb0acdea65ab6a3933e4fac741bc14",
  "js/src/rendezvous.ts": "de71b8f1caff41ce241e4955fb8bfbbc",
  "js/src/style.css": "b22cd99b0a649b4da3b2491ca938c2bd",
  "js/src/ui.js": "13863bf5fcc6c4fa123ac40501fc12c8",
  "js/src/vite-env.d.ts": "db266f67270495e5478804f291146e99",
  "js/src/websock.ts": "b8cb6466b6010089d2e419e7ff403890",
  "js/ts_proto.py": "475d0b5de9bef5e1de20cf116b80bacd",
  "js/tsconfig.json": "f86d42481984659e240ce7b6c6648fcb",
  "js/vite.config.js": "42132f5fd99e75b4025408f9050436ec",
  "js/yarn.lock": "21422deffdcd161975f986de506221cd",
  "libopus.js": "b58c2084b852e6df5ec89fd16adcf920",
  "libopus.wasm": "b8801d4a953d58e739fd9d25134467d3",
  "libs/firebase-analytics.js": "c6fcb4296681957bc1dd5fcd8e7eee50",
  "libs/firebase-app.js": "f61459b893c6422d6e8e96e8aa5fbdcd",
  "main.dart.js": "b036a40f4ef560038fc71de60d3b0242",
  "manifest.json": "18e53390c166023eb5b32e95b324ec2b",
  "ogvjs-1.8.6/COPYING": "d35e32d19d3d58a88cd0bc98e94a9073",
  "ogvjs-1.8.6/COPYING-dav1d.txt": "c8055cfe7548dfdaa3a6dc45d8793669",
  "ogvjs-1.8.6/COPYING-ogg.txt": "db1b7a668b2a6f47b2af88fb008ad555",
  "ogvjs-1.8.6/COPYING-opus.txt": "e304cdf74c2a1b0a33a5084c128a23a3",
  "ogvjs-1.8.6/COPYING-theora.txt": "cf91718f59eb6a83d06dc7bcaf411132",
  "ogvjs-1.8.6/COPYING-vorbis.txt": "70c7063491d2d9f76a098d62ed5134f1",
  "ogvjs-1.8.6/LICENSE-nestegg.txt": "be7778ed82a56425bc6bef17cc6a52d9",
  "ogvjs-1.8.6/LICENSE-vpx.txt": "d5b04755015be901744a78cc30d390d4",
  "ogvjs-1.8.6/PATENTS-vpx.txt": "c6926d0cb07d296f886ab6e0cc5a85b7",
  "ogvjs-1.8.6/README.md": "31bd3e2e67eb876b15176ca710826b0d",
  "ogvjs-1.8.6/ogv-decoder-audio-opus-wasm.js": "e52215cfa46e79fdd227591cde97cca5",
  "ogvjs-1.8.6/ogv-decoder-audio-opus-wasm.wasm": "d0542d63452d97c839f7e1971d5bfce7",
  "ogvjs-1.8.6/ogv-decoder-audio-vorbis-wasm.js": "4a9d3e7ee82d003c26e7c8e620c47bbd",
  "ogvjs-1.8.6/ogv-decoder-audio-vorbis-wasm.wasm": "822a5fe76af25d4dd03c66e048c1287e",
  "ogvjs-1.8.6/ogv-decoder-video-av1-mt-wasm.js": "6f91e8bf224d76b4265083fb29b00bc1",
  "ogvjs-1.8.6/ogv-decoder-video-av1-mt-wasm.wasm": "927555aeda94b975d139e452cee41d6d",
  "ogvjs-1.8.6/ogv-decoder-video-av1-mt-wasm.worker.js": "0b99a97dab10c334fa856c366b9d2722",
  "ogvjs-1.8.6/ogv-decoder-video-av1-simd-mt-wasm.js": "a903c97fe7e34cbc81afbcf3d1587a76",
  "ogvjs-1.8.6/ogv-decoder-video-av1-simd-mt-wasm.wasm": "4da56c7c168b7555dbb3945cf3f46639",
  "ogvjs-1.8.6/ogv-decoder-video-av1-simd-mt-wasm.worker.js": "45b05a259b695a538aa636fefb8baf5c",
  "ogvjs-1.8.6/ogv-decoder-video-av1-simd-wasm.js": "14e3edde7aab7f4a79399276554bfe4e",
  "ogvjs-1.8.6/ogv-decoder-video-av1-simd-wasm.wasm": "29bb3462a7dea17cccd89b2ea7df5837",
  "ogvjs-1.8.6/ogv-decoder-video-av1-wasm.js": "173c2bd427cb559aec38e544abae77c0",
  "ogvjs-1.8.6/ogv-decoder-video-av1-wasm.wasm": "3a2dadb9246caf39c7bea74b03363092",
  "ogvjs-1.8.6/ogv-decoder-video-theora-wasm.js": "14e2a10fb307f1a85a80f38f67b845c0",
  "ogvjs-1.8.6/ogv-decoder-video-theora-wasm.wasm": "9f6fda12b75d61faf69df090ef3859c6",
  "ogvjs-1.8.6/ogv-decoder-video-vp8-mt-wasm.js": "58f6fa9641b8d3eebe0e421e13cdf019",
  "ogvjs-1.8.6/ogv-decoder-video-vp8-mt-wasm.wasm": "fbc953b8f4baed5c44254b3295446205",
  "ogvjs-1.8.6/ogv-decoder-video-vp8-mt-wasm.worker.js": "322763a048743e2029229e56ccfc1c54",
  "ogvjs-1.8.6/ogv-decoder-video-vp8-wasm.js": "4d36a93e0981d3c4b28fdb756259d60f",
  "ogvjs-1.8.6/ogv-decoder-video-vp8-wasm.wasm": "87c067b881492e09b057a7eec75513e9",
  "ogvjs-1.8.6/ogv-decoder-video-vp9-mt-wasm.js": "3861e7af5c402ab41fcdfe162ade1e5d",
  "ogvjs-1.8.6/ogv-decoder-video-vp9-mt-wasm.wasm": "3bd7aa3cd5aab801b11c7268da2f0cb8",
  "ogvjs-1.8.6/ogv-decoder-video-vp9-mt-wasm.worker.js": "af7a85e4a36dfd859fc69405f09b1428",
  "ogvjs-1.8.6/ogv-decoder-video-vp9-simd-mt-wasm.js": "18471a0789104e680105a952c52629c2",
  "ogvjs-1.8.6/ogv-decoder-video-vp9-simd-mt-wasm.wasm": "1f2de2f92dac705af4027c8f6b3f1412",
  "ogvjs-1.8.6/ogv-decoder-video-vp9-simd-mt-wasm.worker.js": "0fa2850dd1295766d0f5d155933c8f7e",
  "ogvjs-1.8.6/ogv-decoder-video-vp9-simd-wasm.js": "642e20237860ba8169c977838d95c490",
  "ogvjs-1.8.6/ogv-decoder-video-vp9-simd-wasm.wasm": "b69b28fcc982bf9c4e734dc7721a027c",
  "ogvjs-1.8.6/ogv-decoder-video-vp9-wasm.js": "63766c6d6826aa7642d37b963e889b17",
  "ogvjs-1.8.6/ogv-decoder-video-vp9-wasm.wasm": "c4fdab9e6eb18813dc88f0e8cd0003ac",
  "ogvjs-1.8.6/ogv-demuxer-ogg-wasm.js": "d67a1c99133b0732f64163da6efb130b",
  "ogvjs-1.8.6/ogv-demuxer-ogg-wasm.wasm": "e8fa0984a376a86556c2b5793eb9de82",
  "ogvjs-1.8.6/ogv-demuxer-webm-wasm.js": "84b1be40700973a74542c6c952f162b4",
  "ogvjs-1.8.6/ogv-demuxer-webm-wasm.wasm": "304ffe0406cdddab408065d820a0bb1b",
  "ogvjs-1.8.6/ogv-es2017.js": "b1bbdae47f8946e190298b77618a5ba5",
  "ogvjs-1.8.6/ogv-support.js": "10520c38312b18528b7b26c1e36a97b9",
  "ogvjs-1.8.6/ogv-version.js": "fbe86349102504b4c8abb5a33a1b0800",
  "ogvjs-1.8.6/ogv-worker-audio.js": "2f02797ab21bd3c68e12b90f002d758b",
  "ogvjs-1.8.6/ogv-worker-video.js": "ed81cddf8a74ff4b44eae8ea8a080b32",
  "ogvjs-1.8.6/ogv.js": "a4a2cf98c22e099d08a342ea9521b504",
  "start-server.bat": "eb6bff5aa07dec1afb80e3bdec7f6e41",
  "version.json": "370e398dd1c5880b6f548a978d403ab2",
  "web_deps.tar.gz": "c951441c06a9a9b7b6136c6d459e3899",
  "yarn.lock": "dd07d22cbf9a738bdb68b591bc1bc742",
  "yuv-canvas-1.2.6.js": "c4ace642c725cf918ed9440426b583b8",
  "yuv.js": "5cff03855bc9577b6104b9743ff26790",
  "yuv.wasm": "443e8cb7b56fff2550ce0a411f3a05eb",
  "/": "04073c04d7d82076c996fce06ff6a5ef"
};

// The application shell files that are downloaded before a service worker can
// start.
const CORE = [
  "main.dart.js",
"index.html",
"assets/AssetManifest.json",
"assets/FontManifest.json"];
// During install, the TEMP cache is populated with the application shell files.
self.addEventListener("install", (event) => {
  self.skipWaiting();
  return event.waitUntil(
    caches.open(TEMP).then((cache) => {
      return cache.addAll(
        CORE.map((value) => new Request(value, {'cache': 'reload'})));
    })
  );
});

// During activate, the cache is populated with the temp files downloaded in
// install. If this service worker is upgrading from one with a saved
// MANIFEST, then use this to retain unchanged resource files.
self.addEventListener("activate", function(event) {
  return event.waitUntil(async function() {
    try {
      var contentCache = await caches.open(CACHE_NAME);
      var tempCache = await caches.open(TEMP);
      var manifestCache = await caches.open(MANIFEST);
      var manifest = await manifestCache.match('manifest');
      // When there is no prior manifest, clear the entire cache.
      if (!manifest) {
        await caches.delete(CACHE_NAME);
        contentCache = await caches.open(CACHE_NAME);
        for (var request of await tempCache.keys()) {
          var response = await tempCache.match(request);
          await contentCache.put(request, response);
        }
        await caches.delete(TEMP);
        // Save the manifest to make future upgrades efficient.
        await manifestCache.put('manifest', new Response(JSON.stringify(RESOURCES)));
        return;
      }
      var oldManifest = await manifest.json();
      var origin = self.location.origin;
      for (var request of await contentCache.keys()) {
        var key = request.url.substring(origin.length + 1);
        if (key == "") {
          key = "/";
        }
        // If a resource from the old manifest is not in the new cache, or if
        // the MD5 sum has changed, delete it. Otherwise the resource is left
        // in the cache and can be reused by the new service worker.
        if (!RESOURCES[key] || RESOURCES[key] != oldManifest[key]) {
          await contentCache.delete(request);
        }
      }
      // Populate the cache with the app shell TEMP files, potentially overwriting
      // cache files preserved above.
      for (var request of await tempCache.keys()) {
        var response = await tempCache.match(request);
        await contentCache.put(request, response);
      }
      await caches.delete(TEMP);
      // Save the manifest to make future upgrades efficient.
      await manifestCache.put('manifest', new Response(JSON.stringify(RESOURCES)));
      return;
    } catch (err) {
      // On an unhandled exception the state of the cache cannot be guaranteed.
      console.error('Failed to upgrade service worker: ' + err);
      await caches.delete(CACHE_NAME);
      await caches.delete(TEMP);
      await caches.delete(MANIFEST);
    }
  }());
});

// The fetch handler redirects requests for RESOURCE files to the service
// worker cache.
self.addEventListener("fetch", (event) => {
  if (event.request.method !== 'GET') {
    return;
  }
  var origin = self.location.origin;
  var key = event.request.url.substring(origin.length + 1);
  // Redirect URLs to the index.html
  if (key.indexOf('?v=') != -1) {
    key = key.split('?v=')[0];
  }
  if (event.request.url == origin || event.request.url.startsWith(origin + '/#') || key == '') {
    key = '/';
  }
  // If the URL is not the RESOURCE list then return to signal that the
  // browser should take over.
  if (!RESOURCES[key]) {
    return;
  }
  // If the URL is the index.html, perform an online-first request.
  if (key == '/') {
    return onlineFirst(event);
  }
  event.respondWith(caches.open(CACHE_NAME)
    .then((cache) =>  {
      return cache.match(event.request).then((response) => {
        // Either respond with the cached resource, or perform a fetch and
        // lazily populate the cache only if the resource was successfully fetched.
        return response || fetch(event.request).then((response) => {
          if (response && Boolean(response.ok)) {
            cache.put(event.request, response.clone());
          }
          return response;
        });
      })
    })
  );
});

self.addEventListener('message', (event) => {
  // SkipWaiting can be used to immediately activate a waiting service worker.
  // This will also require a page refresh triggered by the main worker.
  if (event.data === 'skipWaiting') {
    self.skipWaiting();
    return;
  }
  if (event.data === 'downloadOffline') {
    downloadOffline();
    return;
  }
});

// Download offline will check the RESOURCES for all files not in the cache
// and populate them.
async function downloadOffline() {
  var resources = [];
  var contentCache = await caches.open(CACHE_NAME);
  var currentContent = {};
  for (var request of await contentCache.keys()) {
    var key = request.url.substring(origin.length + 1);
    if (key == "") {
      key = "/";
    }
    currentContent[key] = true;
  }
  for (var resourceKey of Object.keys(RESOURCES)) {
    if (!currentContent[resourceKey]) {
      resources.push(resourceKey);
    }
  }
  return contentCache.addAll(resources);
}

// Attempt to download the resource online before falling back to
// the offline cache.
function onlineFirst(event) {
  return event.respondWith(
    fetch(event.request).then((response) => {
      return caches.open(CACHE_NAME).then((cache) => {
        cache.put(event.request, response.clone());
        return response;
      });
    }).catch((error) => {
      return caches.open(CACHE_NAME).then((cache) => {
        return cache.match(event.request).then((response) => {
          if (response != null) {
            return response;
          }
          throw error;
        });
      });
    })
  );
}
