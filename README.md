# Samspel Hub

Multiroom-hubb för Home Assistant OS. iPhone spelar via AirPlay till hubben, som skickar ljudet vidare till
flera högtalare samtidigt: Google Cast (Google Home, Chromecast) och Denon HEOS. Skriven i Go.

```
iPhone ──AirPlay──▶ shairport-sync (en per grupp och högtalare) ──PCM──▶ ffmpeg ──MP3──▶ HTTP-ström
                                                                                          │
                                       Google Cast ◀── hämtar strömmen ◀─────────────────┤
                                       Denon HEOS  ◀── hämtar strömmen (play_stream) ◀───┘
```

## Funktioner
- **Grupper:** skapa grupper av högtalare i webbgränssnittet. Varje grupp syns som en egen AirPlay-högtalare.
- **Varje högtalare går också att välja** i AirPlay-menyn, under sitt eget namn.
- **Synk:** Cast och HEOS buffrar olika länge, så varje enhet i en grupp har en egen fördröjning (i sekunder) som du ställer in efter gehör. Testtonen (ett pip per sekund) är gjord för det.
- **Volym:** telefonens volymreglage sätter högtalarnas egen volym direkt, med justerbar nivå per rum (balans).
- **Spelas nu:** titel, artist, album och omslag visas i gränssnittet (visar vad telefonen spelar just nu, alltså före högtalarnas buffer).
- Enheter upptäcks vid start och var tionde minut, och sparas.

## Installera
1. Home Assistant: Inställningar → Tillägg → Tilläggsbutik → ⋮ → Förråd → lägg till `https://github.com/areskoug/DCS3020L`.
2. Installera **Samspel Hub** och starta. Gränssnittet finns på `http://<ha-ip>:8080`.
3. Skapa en grupp, tryck **Testton** och ställ in fördröjningen per enhet tills pipen låter som ett enda ljud.
   Fördröjningen gäller först när gruppen startas om.
4. Välj gruppens eller högtalarens namn i AirPlay-menyn på telefonen.

HA läser den gren som är förvald i GitHub. Höj `version` i `samspel/config.yaml` för att HA ska erbjuda en uppdatering.

## Begränsningar
- **Fördröjning:** högtalarna buffrar ungefär 10 s (HEOS) och 26 s (Cast) innan de spelar. Därför kommer ljudet sent,
  och paus, låtbyte och tryck på play märks sent. Det beror på högtalarna. `LIVE`-läge i Cast och andra ljudformat gav ingen förbättring.
- Ljudet kodas som MP3 192 kbit/s, eller som okomprimerad WAV (valbart per grupp, testas för lägre lagg). Fördröjningarna sparas separat per format.
- AirPlay 1 (Alpines shairport-sync saknar AirPlay 2 och nqptp). Räcker för musik från iPhone.
- Om en högtalare ingår i en aktiv grupp och du spelar till den enskilt vinner den som startade senast.
- Hubben har ingen inloggning. Den ska bara nås från det lokala nätet.

## Utveckling
Kräver Go 1.25 och ffmpeg.

    cd samspel
    go test ./...
    go run ./cmd/hub -port 8080        # utan shairport-sync fungerar allt utom AirPlay

- `internal/devices`: upptäckt och styrning (Cast via mDNS, HEOS via SSDP och CLI på port 1255).
- `internal/stream`: ffmpeg-kodning, tidsstämplade fördröjningar och testsignal.
- `internal/airplay`: en shairport-sync-process per grupp, ljud- och metadata-pipes.
- `internal/hub`: grupper, enkelgrupper per enhet, volym och uppspelning.
- `internal/web`: API, ljudströmmar och gränssnitt.
- `samspel/third_party/go-chromecast`: kopia av [vishen/go-chromecast](https://github.com/vishen/go-chromecast) (Apache-2.0). Enda ändringen är att strömmen skickas som `LIVE`.
