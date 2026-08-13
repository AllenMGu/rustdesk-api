window._gwen = {}
window._gwen.kv = {}
const apiserver = localStorage.getItem('api-server')

function stringToUint8Array(str) {
    var arr = [];
    for (var i = 0, j = str.length; i < j; ++i) {
        arr.push(str.charCodeAt(i));
    }

    var tmpUint8Array = new Uint8Array(arr);
    return tmpUint8Array
}

function getQueryVariable() {
    const query = window.location.hash.substring(3);
    const vars = query.split("&");
    for (var i = 0; i < vars.length; i++) {
        var pair = vars[i].split("=");
        window._gwen.kv[pair[0]] = pair[1]
    }
}

getQueryVariable()

const id = window._gwen.kv.id || ''
if (id) {
    localStorage.setItem('remote-id', id)
}
const share_token = window._gwen.kv.share_token || ''
if (share_token) {
    fetch(apiserver + "/api/shared-peer", {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({share_token})
    }).then(res => res.json()).then(res => {
        if (res.code === 0) {
            localStorage.setItem('custom-rendezvous-server', res.data.id_server)
            localStorage.setItem('key', res.data.key)
            const peer = res.data.peer
            localStorage.setItem('remote-id', peer.info.id)
            peer.tmppwd = stringToUint8Array(window.atob(peer.tmppwd)).toString()
            const oldPeers = JSON.parse(localStorage.getItem('peers')) || {}
            oldPeers[peer.info.id] = peer
            localStorage.setItem('peers', JSON.stringify(oldPeers))
        }
    })
}

let fetching = false
export function getServerConf(token){
    if(fetching){
        return
    }
    fetching = true
    fetch(apiserver + "/api/server-config", {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': 'Bearer ' + token
            }
        }
    ).then(res => res.json()).then(res => {
        fetching = false
        if (res.code === 0) {
            if (!localStorage.getItem('custom-rendezvous-server') || !localStorage.getItem('key')) {
                localStorage.setItem('custom-rendezvous-server', res.data.id_server)
                localStorage.setItem('key', res.data.key)
            }
            const books = Array.isArray(res.data.address_books) ? res.data.address_books : []
            const oldBooks = localStorage.getItem('webclient-address-books') || '[]'
            const newBooks = JSON.stringify(books)
            let catalogChanged = oldBooks !== newBooks
            localStorage.setItem('webclient-address-books', newBooks)
            if (res.data.peers) {
                const oldPeers = JSON.parse(localStorage.getItem('peers')) || {}
                let needUpdate = false
                const serverPeerIds = new Set(Object.keys(res.data.peers))
                Object.keys(oldPeers).forEach(k => {
                    if (!serverPeerIds.has(k) && oldPeers[k].address_book) {
                        oldPeers[k].address_book = false
                        oldPeers[k].address_books = []
                        oldPeers[k].address_book_details = {}
                        if (oldPeers[k].address_book_password) {
                            delete oldPeers[k].password
                            delete oldPeers[k].address_book_password
                            oldPeers[k].remember = false
                        }
                        if (oldPeers[k].info) {
                            delete oldPeers[k].info.hash
                        }
                        if (!oldPeers[k].managed && !oldPeers[k].last_connected) {
                            delete oldPeers[k]
                        }
                        needUpdate = true
                    }
                })
                Object.keys(res.data.peers).forEach(k => {
                    if (!oldPeers[k]) {
                        oldPeers[k] = res.data.peers[k]
                        needUpdate = true
                    } else {
                        const before = JSON.stringify({
                            info: oldPeers[k].info,
                            address_book: oldPeers[k].address_book,
                            address_books: oldPeers[k].address_books,
                            address_book_details: oldPeers[k].address_book_details,
                            managed: oldPeers[k].managed,
                        })
                        oldPeers[k].info = res.data.peers[k].info
                        oldPeers[k].address_book = Boolean(res.data.peers[k].address_book)
                        oldPeers[k].address_books = Array.isArray(res.data.peers[k].address_books) ? res.data.peers[k].address_books : []
                        oldPeers[k].address_book_details = res.data.peers[k].address_book_details || {}
                        oldPeers[k].managed = Boolean(res.data.peers[k].managed)
                        if (!oldPeers[k].address_book && oldPeers[k].address_book_password) {
                            delete oldPeers[k].password
                            delete oldPeers[k].address_book_password
                            oldPeers[k].remember = false
                        }
                        const after = JSON.stringify({
                            info: oldPeers[k].info,
                            address_book: oldPeers[k].address_book,
                            address_books: oldPeers[k].address_books,
                            address_book_details: oldPeers[k].address_book_details,
                            managed: oldPeers[k].managed,
                        })
                        if (before !== after) {
                            needUpdate = true
                        }
                    }
                    if (oldPeers[k].info && oldPeers[k].info.hash && !oldPeers[k].password) {
                        let p1 = window.atob(oldPeers[k].info.hash)
                        const pwd = stringToUint8Array(p1)
                        oldPeers[k].password = pwd.toString()
                        oldPeers[k].remember = true
                        oldPeers[k].address_book_password = true
                    }
                })
                localStorage.setItem('peers', JSON.stringify(oldPeers))
                // A reload tears down the active relay WebSocket. In particular,
                // handlePeerInfo() updates the local peer immediately after login,
                // so a concurrent server-config response can otherwise reload the
                // page just as the first video frame arrives.
                if ((needUpdate || catalogChanged) && !window.curConn) {
                    window.location.reload()
                }
            }
        }
    }).catch(_ => {
        fetching = false
    })
}
