# AI GIF Maker CLI image

Turn text into animated GIFs with AI GIF Maker. Native GIF generation — no video conversion. Type what you want and download your GIF in about a minute.

This image is the official command-line tool for [AI GIF Maker](https://aigifmaker.ai/?utm_source=dockerhub&utm_medium=package): pass a prompt, get back the URL of a real animated GIF. Handy in scripts, CI jobs and chat bots.

```bash
docker run --rm -e AIGIFMAKER_API_KEY san8888/aigifmaker "a cat dancing in the rain, pixel art"
# prints the GIF URL
```

Each run submits the prompt, waits for the GIF and prints its URL.

## Install

```bash
docker pull san8888/aigifmaker
```

A static Go binary on an empty base image, for `linux/amd64` and `linux/arm64`. Exit codes: 0 ok, 1 failure, 2 usage, 3 key problem, 4 out of credits.

## API key

Sign in, create a key at https://aigifmaker.ai/settings/apikeys, then export it and pass it through:

```bash
export AIGIFMAKER_API_KEY=sk-...
```

Each GIF costs 1 credit on your AI GIF Maker account, the same as on the website. New accounts get one free GIF (it carries a small aigifmaker.ai badge). Failed generations are refunded. Credits are on the [pricing](https://aigifmaker.ai/pricing?utm_source=dockerhub&utm_medium=package) page.

## Submit now, collect later

```bash
docker run --rm -e AIGIFMAKER_API_KEY san8888/aigifmaker -no-wait "a rocket launching, retro cartoon"   # prints the task id
docker run --rm -e AIGIFMAKER_API_KEY san8888/aigifmaker -status <id>                                  # JSON: status, images, queued
docker run --rm -e AIGIFMAKER_API_KEY san8888/aigifmaker credits                                       # credits left, free to call
```

A run waits up to 15 minutes by default, polling every 3 seconds; change that with `-timeout 10m`. On an account that has not bought credits yet, a GIF can wait in a queue first (`"queued":true` with `queueEta` in seconds); the CLI keeps polling through it.

## Errors

Errors print to stderr prefixed with their kind:

- `auth`: no key, or the key is wrong or deleted
- `quota`: the account is out of credits
- `rejected`: moderation refused the prompt, or the account is cooling down after a refusal; reword rather than retry
- `failed`: the GIF was accepted and then failed; the credit comes back
- `timeout`: the wait ran out; the GIF may still finish in your history on the site
- `other`: anything else, with the service's message

## Support

support@aigifmaker.ai

MIT licensed.
