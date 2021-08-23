FROM nikolaik/python-nodejs@sha256:215bb6c4992613a9ec69ad372578991ed184de11b1ef33520491bc898e3fe936

COPY ./contracts /eth_wit/contracts
COPY ./tests /eth_wit/tests
COPY Pipfile* /eth_wit/
COPY yarn.lock package.json /eth_wit/

WORKDIR /eth_wit

RUN pipenv install --deploy --system
RUN yarn install
ENV PATH="./node_modules/.bin:${PATH}"

CMD ["pipenv", "run", "brownie", "test"]
