"""Log network requests of amtui's hidden page for N seconds, grouped by type/host."""
import asyncio, json, sys, collections, urllib.parse, websockets
sys.path.insert(0, __import__("os").path.dirname(__file__))
from drive import ws_url
async def main(secs):
    by = collections.Counter(); size = collections.Counter()
    async with websockets.connect(ws_url(), max_size=None) as ws:
        await ws.send(json.dumps({"id": 1, "method": "Network.enable"}))
        types = {}
        end = asyncio.get_event_loop().time() + secs
        while (left := end - asyncio.get_event_loop().time()) > 0:
            try: m = json.loads(await asyncio.wait_for(ws.recv(), left))
            except asyncio.TimeoutError: break
            p = m.get("params", {})
            if m.get("method") == "Network.requestWillBeSent":
                host = urllib.parse.urlparse(p["request"]["url"]).netloc or p["request"]["url"][:20]
                k = (p.get("type", "?"), host); types[p["requestId"]] = k; by[k] += 1
            elif m.get("method") == "Network.loadingFinished" and p["requestId"] in types:
                size[types[p["requestId"]]] += p.get("encodedDataLength", 0)
    for k, n in by.most_common(): print(f"{n:4d} req {size[k]/1024:9.0f} KB  {k[0]:10s} {k[1]}")
asyncio.run(main(float(sys.argv[1])))
