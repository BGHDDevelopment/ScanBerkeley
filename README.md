# ScanBerkeley

This repository is a home for all the config files and modified software to run the now shutdown [@ScanBerkeley](https://twitter.com/ScanBerkeley) radio and transcription systems.

This is not a drag-and-drop setup. I am only providing the config files and modified files for how we ran the system. Each will need to be installed individually from the proper software locations (linked below). This is not a beginner setup and all the systems require knowledge in using SDR's and docker. Configs for NGINX setup are not being provided, if you are even attempting this I am going to assume you know how to run a website.
## Needed Software

[Trunk-Recorder](https://github.com/robotastic/trunk-recorder)

[Rdio-Scanner](https://github.com/chuot/rdio-scanner)

[Trunk-Transcribe](https://github.com/CrimeIsDown/trunk-transcribe)

[CrimeIsDown](https://github.com/CrimeIsDown/crimeisdown-v3)

## Needed Hardware

I ran this system using both a raspberry pi 4 for Trunk-Recorder and Rdio-Scanner, along with a DigitalOcean droplet for Trunk-Transcribe and crimeisdown-V3. We used 3x of [this SDR and antena](https://www.amazon.com/dp/B01GDN1T4S) that seemed to work fine for the local connection to the raspberry pi.

## Notice:
All these files have aspects like API keys and URLs set as: `REDACTED_CHANGEME`

You will need to change these to your own software values.



## Trunk-Recorder    

| File | Description                |
| :-------- | :------------------------- |
| `config.json` | Contains the frequiences and settings for upload scripts for Rdio-Scanner and Trunk-Transcribe. |
| `talkgroups.csv` | Contains the talk groups that the system will record. |
| `UnitTags.csv` | Contains the known Berkeley unit tags (Radio IDs). |
| `transcribe.sh` | Upload script designed to send the recordings to Trunk-Transcribe on another server. |

## Trunk-Transcribe   

| File | Description                |
| :-------- | :------------------------- |
| `/config/notifications.json` | Contains the phrases to send via notifications on Slack or other platforms. |
| `/app/whisper.py` | Contains a modified file for the OpenAI prompt. |
| `.env` | Contains all main Trunk-Transcribe settings. All URLs/API Keys we modified are redacted. |


## crimeisdown-V3   

| File | Description                |
| :-------- | :------------------------- |
| `/config/environment.js` | Contains the main API urls (redacted). |
| `/app/*` | Contains all our modified files for the web interface. This removes all but the transcribe page. |
