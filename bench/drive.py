"""Tiny CDP driver for benchmarking amtui: talks to amtui's hidden Chrome."""
import asyncio, json, sys, time, urllib.request, os, websockets
PORTFILE = os.path.expanduser("~/.config/amtui/chrome/DevToolsActivePort")

def ws_url():
    port = open(PORTFILE).read().split()[0]
    tabs = json.load(urllib.request.urlopen(f"http://localhost:{port}/json/list", timeout=2))
    return [t for t in tabs if t["type"] == "page" and "music.apple.com" in t["url"]][0]["webSocketDebuggerUrl"]

async def ev(js):
    async with websockets.connect(ws_url(), max_size=None) as ws:
        await ws.send(json.dumps({"id": 1, "method": "Runtime.evaluate",
            "params": {"expression": js, "returnByValue": True, "awaitPromise": True}}))
        while True:
            m = json.loads(await ws.recv())
            if m.get("id") == 1:
                return m["result"]["result"].get("value")

CMDS = {
 "authed": "(()=>{try{return MusicKit.getInstance().isAuthorized===true}catch(e){return false}})()",
 "state": "(()=>{const mk=MusicKit.getInstance();return JSON.stringify({st:mk.playbackState,t:mk.currentPlaybackTime,pos:mk.queue&&mk.queue.position,len:mk.queue&&mk.queue.length,vol:mk.volume})})()",
 "mute": "(()=>{MusicKit.getInstance().volume=0;return true})()",
 "pause": "(()=>{MusicKit.getInstance().pause();return true})()",
 "play": "(()=>{MusicKit.getInstance().play();return true})()",
}
PLAY_ALBUM = """(async()=>{const mk=MusicKit.getInstance();mk.volume=0;
 const r=await mk.api.music('/v1/catalog/'+(mk.storefrontId||'us')+'/search',{term:%s,types:'albums',limit:1});
 const id=r.data.results.albums.data[0].id; await mk.setQueue({album:id,startPlaying:true}); mk.volume=0; return id})()"""

def main():
    cmd = sys.argv[1]
    if cmd == "wait-authed":
        deadline = time.time() + float(sys.argv[2] if len(sys.argv) > 2 else 90)
        while time.time() < deadline:
            try:
                if asyncio.run(ev(CMDS["authed"])): print("authed"); return
            except Exception: pass
            time.sleep(1)
        sys.exit("timeout waiting for authed")
    if cmd == "play-album":
        print(asyncio.run(ev(PLAY_ALBUM % json.dumps(sys.argv[2])))); return
    if cmd == "eval":
        print(asyncio.run(ev(sys.argv[2]))); return
    print(asyncio.run(ev(CMDS[cmd])))

main()
