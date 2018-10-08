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

### Options
`-i`      Manifest (m3u8) url to download  
`-o`      Path or URI where the files will be stored (local path or S3 bucket in the format `s3://<bucket>/<path>`  
`-w`      Number of workers to execute concurrent operations. Default: `3`, Min: `1`, Max: `10`

Note: This is a work in progress.

### TODO

Implement FTP delivery (as FileSystem provider)

#### Based on:
[https://github.com/grafov/m3u8](https://github.com/grafov/m3u8)

#### Inspired by:
[https://github.com/kz26/gohls](https://github.com/kz26/gohls)  
[https://github.com/somombo/hlsdownloader](https://github.com/somombo/hlsdownloader)