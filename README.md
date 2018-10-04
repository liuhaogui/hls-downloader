### [WIP] HLS Downloader
Tool written in Go to download HLS streams given its manifest url (m3u8). Streams can be stored in a local folder or delivered to an S3 bucket

### Usage
Using local folder as output:
```
hls-downloader -i https://example.com/hls/master.m3u8 -o /path/to/storage
```
Using S3 bucket as output:
```
hls-downloader -i https://example.com/hls/master.m3u8 -o s3://<bucket>/<path>
```

Note: This is a work in progress.

#### Based on:
[https://github.com/grafov/m3u8](https://github.com/grafov/m3u8)

#### Inspired by:
[https://github.com/kz26/gohls](https://github.com/kz26/gohls)  
[https://github.com/somombo/hlsdownloader](https://github.com/somombo/hlsdownloader)