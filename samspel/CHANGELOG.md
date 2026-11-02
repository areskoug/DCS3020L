# Ändringslogg

## 0.6.9
- Byggen går snabbare: beroenden kompileras inte om från början för varje version.
- Ändringslogg läggs till. Tester kontrollerar nu att `config.yaml`, ändringsloggen och Dockerfilen är giltiga innan en version släpps.

## 0.6.8
- Tillägget avslutas rent när Home Assistant stoppar det (ingen varning om SIGTERM längre).

## 0.6.7
- Rättar `webui`-länken så att Home Assistant accepterar den.

## 0.6.6
- Alla högtalare får 5 sekunders förråd vid start som standard. En HEOS-högtalare var tyst på 3 sekunder.

## 0.6.5
- Misslyckad version: länken till webbgränssnittet hade fel format, så Home Assistant dolde tillägget. Använd 0.6.7 eller senare.

## 0.6.4
- Cast-högtalare får 5 sekunders förråd som standard (3 sekunder gav ibland ett stopp vid start).

## 0.6.3
- Fördröjning per enhet kan ställas in i steg om 0,01 sekunder.

## 0.6.2
- Nytt testljud (dubbelpip och enkelpip var åttonde sekund) så att skillnader på hela sekunder hörs.

## 0.6.1
- Inställbart förråd vid start per högtalare.

## 0.6.0
- Lagget är nere på ca 5 sekunder (var 18–26). Högtalarna får tystnad framför ljudet i stället för att ljudet hålls tillbaka, så de stannar inte och väntar vid start.
- Fördröjning per enhet görs på samma sätt.

## 0.5.0
- Experiment med att hoppa fram i Cast-bufferten. Togs bort i 0.6.0.
- Loggar när Cast börjar spela eller stannar och buffrar.

## 0.4.2
- Ljudformat (MP3 eller WAV) kan väljas per högtalare när den spelas ensam.

## 0.4.1
- WAV som alternativ med lägre lagg, valbart per grupp. Fördröjningar sparas per format.

## 0.4.0
- Spelas nu: titel, artist, album och omslag visas.
- Volymbalans per rum.
- Tester och README.

## 0.3.2
- Valet av ljudformat togs bort tillfälligt.

## 0.3.1
- Varje högtalare går att välja i AirPlay-menyn, inte bara grupper.
- Enheter upptäcks när tillägget startar och var tionde minut, och sparas.

## 0.2.3
- Loggar volymändringar.

## 0.2.2
- Telefonens volym sätter högtalarnas egen volym direkt, i stället för att ligga i ljudet.

## 0.2.1
- Loggar utdata från shairport-sync och inkommande ljud.

## 0.2.0
- Grupperna syns som AirPlay-högtalare (en shairport-sync per grupp).

## 0.1.6
- Testljud med ett pip per sekund.

## 0.1.4
- Fördröjning per enhet i en grupp.

## 0.1.3
- Cast får strömmen som direktsändning.

## 0.1.2
- Loggar när en högtalare ansluter till strömmen.

## 0.1.1
- HEOS får webbadressen kodad enligt specen och trafiken loggas.

## 0.1.0
- Första versionen: upptäckt av Google Cast och Denon HEOS, grupper, webbgränssnitt och en testton.
