Currently only the basic functionality work
like compose a mail
read a mail
reply to a mail




replace the credential.json with yours and


## To test out the application 
- Go to google cloud console
- Add a project name it whatver eg. Gmail-TUI
- APIs and Services
- Library
- Gmail API
- Enable create an oauth2 token
- Download the credentials.json
- Replaces the placeholder credentials.json in this project with the downlaoded contents
- ```bash
  go mod tidy
  (OR)
  go install
  ```
- from the project root run
```bash
go run .
```



### Note on downloading attachments recieved from an email -> Check a downloads folder created in the same directory



```bash
go run .
```

and wait till it starts

## how it looks:

![inbox](./images/inbox.png)
![compose](./images/compose.png)
![attachment sent](./images/attach_send.png)
![attachment recieved](./images/attach_rec.png)

### ps first run always takes a bit longer but after that its chill
