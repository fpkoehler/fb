package main

/* User info is stored as XML files in the "users" subdirectory */

import (
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"path/filepath"
	"time"

	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
)

/**********************************************************/

func writeUserFile(user *User) {
	user.fileLock.Lock()
	defer user.fileLock.Unlock()

	/* write user info/sections to file */
	userFileName := "users/" + user.Email + ".xml"
	log.Println("writing to ", userFileName)
	userXMLFile, err := os.Create(userFileName)
	if err != nil {
		log.Println(err.Error())
		return
	}
	defer userXMLFile.Close()

	enc := xml.NewEncoder(userXMLFile)
	enc.Indent("", "    ")
	if err := enc.Encode(user); err != nil {
		log.Println(err.Error())
	}
	log.Println("wrote to ", userFileName)
}

/**********************************************************/

func sendEmail(toUser string, subject string, body string) {
	from := mail.Address{"", options.AdminEmail}
	to := mail.Address{"", toUser}

	// Setup headers
	headers := make(map[string]string)
	headers["From"] = from.String()
	headers["To"] = to.String()
	headers["Subject"] = subject

	// Setup message
	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + body

	// Connect to the SMTP Server
	servername := "smtp.aol.com:465"
	//	servername := "smtp.1and1.com:465"

	host, _, _ := net.SplitHostPort(servername)

	auth := smtp.PlainAuth("", options.AdminEmail, options.AdminEmailPw, host)

	// TLS config
	tlsconfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	}

	// Here is the key, you need to call tls.Dial instead of smtp.Dial
	// for smtp servers running on 465 that require an ssl connection
	// from the very beginning (no starttls)
	conn, err := tls.Dial("tcp", servername, tlsconfig)
	if err != nil {
		log.Panic(err)
	}

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		log.Panic(err)
	}
	defer c.Quit()

	// Auth
	if err = c.Auth(auth); err != nil {
		log.Panic(err)
	}

	// To && From
	if err = c.Mail(from.Address); err != nil {
		log.Panic(err)
	}

	if err = c.Rcpt(to.Address); err != nil {
		log.Panic(err)
	}

	// SECOND RECEPIENT!!
	// if err = c.Rcpt("fred@foo.com"); err != nil {
	// 	log.Panic(err)
	// }

	// Data
	w, err := c.Data()
	if err != nil {
		log.Println("email c.Data", err.Error())
		return
	}

	_, err = w.Write([]byte(message))
	if err != nil {
		log.Println("email w.Write", err.Error())
		return
	}

	err = w.Close()
	if err != nil {
		log.Println("email w.Close", err.Error())
		return
	}
}

/**********************************************************/

func userWalk(path string, info os.FileInfo, err error) error {
	if info.IsDir() {
		return nil
	}

	if err != nil {
		log.Println("user walk error, ", path, ":", err.Error())
		return nil
	}

	log.Println("loading", path)

	/* http://stackoverflow.com/questions/1821811/how-to-read-write-from-to-file
	 * xml.Unmarshal()'s first argument is a []byte.  A little scary that the only
	 * way to get a []byte from the file is to read the entire file.
	 */
	b, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Println(err.Error())
		return nil
	}

	var user User
	err = xml.Unmarshal(b, &user)
	if err != nil {
		log.Printf("error: %v\n", err)
		return nil
	}

	users[user.Email] = &user

	return nil
}

func getUsers() {

	/* Init global map */
	users = make(map[string]*User)

	err := filepath.Walk("users", userWalk)
	if err != nil {
		fmt.Println("Error getting user info:", err.Error())
		return
	}
}

/**********************************************************/

/**********************************************************/
/* Password Recovery                                      */
/**********************************************************/

const (
	decodedMinLength = 4 /*expiration*/ + 1 /*login*/ + 32 /*signature*/
	decodedMaxLength = 1024                                /* maximum decoded length, for safety */
)

// MinLength is the minimum allowed length of cookie string.
//
// It is useful for avoiding DoS attacks with too long cookies: before passing
// a cookie to Parse or Login functions, check that it has length less than the
// [maximum login length allowed in your application] + MinLength.
var MinLength = base64.URLEncoding.EncodedLen(decodedMinLength)

func getSignature(b []byte, secret []byte) []byte {
	keym := hmac.New(sha256.New, secret)
	keym.Write(b)
	m := hmac.New(sha256.New, keym.Sum(nil))
	m.Write(b)
	return m.Sum(nil)
}

var (
	ErrMalformedCookie = errors.New("malformed cookie")
	ErrWrongSignature  = errors.New("wrong cookie signature")
)

// New returns a signed authentication cookie for the given login,
// expiration time, and secret key.
// If the login is empty, the function returns an empty string.
func New(login string, expires time.Time, secret []byte) string {
	if login == "" {
		return ""
	}
	llen := len(login)
	b := make([]byte, llen+4+32)
	// Put expiration time.
	binary.BigEndian.PutUint32(b, uint32(expires.Unix()))
	// Put login.
	copy(b[4:], []byte(login))
	// Calculate and put signature.
	sig := getSignature([]byte(b[:4+llen]), secret)
	copy(b[4+llen:], sig)
	// Base64-encode.
	return base64.URLEncoding.EncodeToString(b)
}

// NewSinceNow returns a signed authetication cookie for the given login,
// duration since current time, and secret key.
func NewSinceNow(login string, dur time.Duration, secret []byte) string {
	return New(login, time.Now().Add(dur), secret)
}

// Parse verifies the given cookie with the secret key and returns login and
// expiration time extracted from the cookie. If the cookie fails verification
// or is not well-formed, the function returns an error.
//
// Callers must:
//
// 1. Check for the returned error and deny access if it's present.
//
// 2. Check the returned expiration time and deny access if it's in the past.
//
func Parse(cookie string, secret []byte) (login string, expires time.Time, err error) {
	blen := base64.URLEncoding.DecodedLen(len(cookie))
	// Avoid allocation if cookie is too short or too long.
	if blen < decodedMinLength || blen > decodedMaxLength {
		err = ErrMalformedCookie
		return
	}
	b, err := base64.URLEncoding.DecodeString(cookie)
	if err != nil {
		return
	}
	// Decoded length may be different from max length, which
	// we allocated, so check it, and set new length for b.
	blen = len(b)
	if blen < decodedMinLength {
		err = ErrMalformedCookie
		return
	}
	b = b[:blen]

	sig := b[blen-32:]
	data := b[:blen-32]

	realSig := getSignature(data, secret)
	if subtle.ConstantTimeCompare(realSig, sig) != 1 {
		err = ErrWrongSignature
		return
	}
	expires = time.Unix(int64(binary.BigEndian.Uint32(data[:4])), 0)
	login = string(data[4:])
	return
}

// Login returns a valid login extracted from the given cookie and verified
// using the given secret key.  If verification fails or the cookie expired,
// the function returns an empty string.
func Login(cookie string, secret []byte) string {
	l, exp, err := Parse(cookie, secret)
	if err != nil || exp.Before(time.Now()) {
		return ""
	}
	return l
}
