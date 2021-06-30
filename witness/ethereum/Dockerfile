FROM nikolaik/python-nodejs@sha256:215bb6c4992613a9ec69ad372578991ed184de11b1ef33520491bc898e3fe936

COPY ./contracts /eth_wit/contracts
COPY ./tests /eth_wit/tests
COPY Pipfile* /eth_wit/

WORKDIR /eth_wit

# Maybe a way to do this with determinism?
# Does pip support verifying hashes of installed packages?
RUN pip install pipenv
RUN pipenv install --deploy --system
RUN yarn global add ganache-cli

CMD ["pipenv", "run", "brownie", "test"]
