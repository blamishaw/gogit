# GoGit

A git client written in Go. Written as an exercise to learn the Go language.

<hr>

## Usage

GoGit works essentially like (a less sophisticated) git. To use it like git, you'll need to run the following which: builds the project, sets the proper executable permission, and adds it to `/usr/local/bin`:

```
$ make install
```

Now, you can navigate to any directory and run:

```
$ gogit init
```

to initialize a new gogit repository and begin tracking your changes! Inspect the `.gogit` directory initialization creates to see how the changes work.

## Credits

Heavily inspired by the [Git Internals](https://www.leshenko.net/p/ugit/) tutorial from leshenko
