# Cue-maker

Make [CUE sheet](https://en.wikipedia.org/wiki/Cue_sheet_%28computing%29) from tracks or split single sound file to multiple tracks. It requires `ffprobe` utility from [ffmpeg](https://ffmpeg.org).

## Make CUE file from tracks

The following command creates file.cue with all WAV files in current directory:
```
cue-maker cue -o file.cue *.wav
```

You can join and compress (with [OPUS codec](https://opus-codec.org) in this example) all WAV files with `ffmpeg`:
```
printf "file %q\n" *.wav > list.txt
ffmpeg -f concat -safe 0 -i list.txt -vn -dn -map_metadata -1 -acodec libopus -b:a 256000 -ac 2 OUTPUT.mka
```

Edit `file.cue` and replace `FILE` field with actual file name.

## Split single sound file to multiple tracks

Generate labels file from CUE sheet:
```
cue-maker label -i INPUT.cue -o label.txt
```

Open sound file with [Audacity](https://www.audacityteam.org) and import `label.txt` into it.
Export multiple files.

For additional usage details see:
```
cue-maker -h
cue-maker cue -h
cue-maker label -h
```

## Build

```
git clone https://github.com/vs022/cue-maker.git
cd cue-maker
go mod init cue-maker
go build
```
