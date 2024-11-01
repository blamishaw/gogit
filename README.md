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

### A Note on Testing

There are an uncomprehensive set of tests for the CLI commands in `main_test.go`. It has proven tricky to test this for two reasons, namely that essentially all of the commands do some form of file manipulation and that many of the commands are intended to be used in-sequence (e.g. you need to add files to the index before committing).

I've tried to mock a number of (simple) states such that each command can be tested in isolation. Having said that, there are obviously many many directory structures and gogit configurations such that ennumarating them all as tests is infeasible. If you encounter any bugs, please don't hesitate to reach out.

## Credits

Heavily inspired by the [Git Internals](https://www.leshenko.net/p/ugit/) tutorial from leshenko.
